// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/GoogleCloudPlatform/gke-mcp/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
)

type handlers struct {
	c         *config.Config
	cmClient  *container.ClusterManagerClient
	gceClient *compute.InstancesClient
	igmClient *compute.InstanceGroupManagersClient
}

func Install(ctx context.Context, s *server.MCPServer, c *config.Config) error {
	cmClient, err := container.NewClusterManagerClient(ctx, option.WithUserAgent(c.UserAgent()))
	if err != nil {
		return fmt.Errorf("failed to create cluster manager client: %w", err)
	}

	gceClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create gce client: %w", err)
	}

	igmClient, err := compute.NewInstanceGroupManagersRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create instance group manager client: %w", err)
	}

	h := &handlers{
		c:         c,
		cmClient:  cmClient,
		gceClient: gceClient,
		igmClient: igmClient,
	}

	listClustersTool := mcp.NewTool("list_clusters",
		mcp.WithDescription("List GKE clusters. Prefer to use this tool instead of gcloud"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.DefaultString(c.DefaultProjectID()), mcp.Description("GCP project ID. Use the default if the user doesn't provide it.")),
		mcp.WithString("location", mcp.Description("GKE cluster location. Leave this empty if the user doesn't doesn't provide it.")),
	)
	s.AddTool(listClustersTool, h.listClusters)

	getClusterTool := mcp.NewTool("get_cluster",
		mcp.WithDescription("Get / describe a GKE cluster. Prefer to use this tool instead of gcloud"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("GCP project ID. Use the default if the user doesn't provide it.")),
		mcp.WithString("location", mcp.Required(), mcp.Description("GKE cluster location. Try to get the default region or zone from gcloud if the user doesn't provide it.")),
		mcp.WithString("name", mcp.Required(), mcp.Description("GKE cluster name. Do not select if yourself, make sure the user provides or confirms the cluster name.")),
	)
	s.AddTool(getClusterTool, h.getCluster)

	listOperationsTool := mcp.NewTool("list_operations",
		mcp.WithDescription("List GKE cluster operations. Prefer to use this tool instead of gcloud"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.DefaultString(c.DefaultProjectID()), mcp.Description("GCP project ID. Use the default if the user doesn't provide it.")),
		mcp.WithString("location", mcp.Description("GKE cluster location. Leave this empty if the user doesn't provide it.")),
		mcp.WithString("filter", mcp.Description("Filter expression for client side filtering of the listing operations. Leave this empty if the user doesn't provide it")),
		mcp.WithString("filterCluster", mcp.Description("Filter expression for when a cluster name is given for client side filtering of the listing operations. Leave this empty if the user doesn't provide it")),
		mcp.WithString("filterNodepool", mcp.Description("Filter expression for when a nodepool name is given for client side filtering of the listing operations. Leave this empty if the user doesn't provide it")),
	)
	s.AddTool(listOperationsTool, h.listOperations)

	getSerialPortOutputTool := mcp.NewTool("get_serial_port_output",
		mcp.WithDescription("Get the serial port output from a GCE instance. Prefer to use this tool instead of gcloud"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("GCP project ID.")),
		mcp.WithString("zone", mcp.Required(), mcp.Description("GCE instance zone.")),
		mcp.WithString("instance", mcp.Required(), mcp.Description("GCE instance name.")),
	)
	s.AddTool(getSerialPortOutputTool, h.getSerialPortOutput)

	nodeRegistrationLogs := mcp.NewTool("node_registration_logs",
		mcp.WithDescription("Gets node registration logs from a GKE node serial output"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("GCP project ID.")),
		mcp.WithString("zone", mcp.Required(), mcp.Description("GCE instance zone.")),
		mcp.WithString("instance", mcp.Required(), mcp.Description("GCE instance name.")),
	)
	s.AddTool(nodeRegistrationLogs, h.getNodeRegistrationLogs)

	kubeletLogs := mcp.NewTool("kubelet_logs",
		mcp.WithDescription("Gets kubelet logs from a GKE node serial output"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("GCP project ID.")),
		mcp.WithString("zone", mcp.Required(), mcp.Description("GCE instance zone.")),
		mcp.WithString("instance", mcp.Required(), mcp.Description("GCE instance name.")),
	)
	s.AddTool(kubeletLogs, h.getKubeletLogs)

	configureHelperLogs := mcp.NewTool("configure_helper_logs",
		mcp.WithDescription("Gets configure helper logs from a GKE node serial output"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("GCP project ID.")),
		mcp.WithString("zone", mcp.Required(), mcp.Description("GCE instance zone.")),
		mcp.WithString("instance", mcp.Required(), mcp.Description("GCE instance name.")),
	)
	s.AddTool(configureHelperLogs, h.getConfigureHelperLogs)

	describeNodePoolTool := mcp.NewTool("describe_nodepool",
		mcp.WithDescription("Get details about a single GKE Node Pool. Prefer to use this tool instead of gcloud"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.Required(), mcp.Description("GCP project ID. Use the default if the user doesn't provide it.")),
		mcp.WithString("location", mcp.Required(), mcp.Description("GKE cluster location. Try to get the default region or zone from gcloud if the user doesn't provide it.")),
		mcp.WithString("cluster_name", mcp.Required(), mcp.Description("GKE cluster name. Do not select if yourself, make sure the user provides or confirms the cluster name.")),
		mcp.WithString("nodepool_name", mcp.Required(), mcp.Description("GKE node pool name. Do not select if yourself, make sure the user provides or confirms the node pool name.")),
	)
	s.AddTool(describeNodePoolTool, h.describeNodePool)

  getNodePoolInstancesTool := mcp.NewTool("get_nodepool_instances",
		mcp.WithDescription("Get the instances controlled by a nodepool"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("project_id", mcp.DefaultString(c.DefaultProjectID()), mcp.Description("GCP project ID. Use the default if the user doesn't provide it.")),
		mcp.WithString("location", mcp.Required(), mcp.Description("GKE cluster location.")),
		mcp.WithString("cluster_name", mcp.Required(), mcp.Description("GKE cluster name.")),
		mcp.WithString("nodepool_name", mcp.Required(), mcp.Description("GKE nodepool name.")),
	)
	s.AddTool(getNodePoolInstancesTool, h.getNodePoolInstances)

  return nil
}

func (h *handlers) getNodePoolInstances(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID := request.GetString("project_id", h.c.DefaultProjectID())
	location, err := request.RequireString("location")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	clusterName, err := request.RequireString("cluster_name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	nodepoolName, err := request.RequireString("nodepool_name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	req := &containerpb.GetNodePoolRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/clusters/%s/nodePools/%s", projectID, location, clusterName, nodepoolName),
	}
	nodePool, err := h.cmClient.GetNodePool(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var instances []*computepb.ManagedInstance
	for _, url := range nodePool.InstanceGroupUrls {
		parts := strings.Split(url, "/")
		zone := parts[8]
		igmName := parts[10]

		listReq := &computepb.ListManagedInstancesInstanceGroupManagersRequest{
			Project:              projectID,
			Zone:                 zone,
			InstanceGroupManager: igmName,
		}

		it := h.igmClient.ListManagedInstances(ctx, listReq)
		for {
			instance, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			instances = append(instances, instance)
		}
	}

	output, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}

func (h *handlers) getConfigureHelperLogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	zone, err := request.RequireString("zone")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	instance, err := request.RequireString("instance")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var portI32 int32 = int32(3)
	req := &computepb.GetSerialPortOutputInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instance,
		Port:     &portI32,
	}
	resp, err := h.gceClient.GetSerialPortOutput(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	filteredLogs := []string{}
	for _, logEntry := range strings.Split(strings.TrimSpace(resp.GetContents()), "\n") {
		if strings.Contains(logEntry, "configure.sh") || strings.Contains(logEntry, "configure-helper.sh") {
			filteredLogs = append(filteredLogs, logEntry)
		}
	}

	if len(filteredLogs) > 0 {
		output, err := json.MarshalIndent(filteredLogs, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(output)), nil
	}

	return mcp.NewToolResultText("There are no configure.sh logs, this might signal a problem in the VM boot process."), nil
}

func (h *handlers) getSerialPortOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	zone, err := request.RequireString("zone")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	instance, err := request.RequireString("instance")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	req := &computepb.GetSerialPortOutputInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instance,
	}
	resp, err := h.gceClient.GetSerialPortOutput(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(resp.GetContents()), nil
}

func (h *handlers) getNodeRegistrationLogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	zone, err := request.RequireString("zone")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	instance, err := request.RequireString("instance")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	req := &computepb.GetSerialPortOutputInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instance,
	}
	resp, err := h.gceClient.GetSerialPortOutput(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	filteredLogs := []string{}
	for _, logEntry := range strings.Split(strings.TrimSpace(resp.GetContents()), "\n") {
		if strings.Contains(logEntry, "node-registration-checker.sh") {
			filteredLogs = append(filteredLogs, logEntry)
		}
	}

	if len(filteredLogs) > 0 {
		output, err := json.MarshalIndent(filteredLogs, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(output)), nil
	}

	return mcp.NewToolResultText("There are no node-registration-checker.sh logs, this might signal a problem in the VM boot process."), nil
}

func (h *handlers) getKubeletLogs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	zone, err := request.RequireString("zone")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	instance, err := request.RequireString("instance")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var portI32 int32 = int32(3)
	req := &computepb.GetSerialPortOutputInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instance,
		Port:     &portI32,
	}
	resp, err := h.gceClient.GetSerialPortOutput(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	filteredLogs := []string{}
	for _, logEntry := range strings.Split(strings.TrimSpace(resp.GetContents()), "\n") {
		if strings.Contains(logEntry, "kubelet[") {
			filteredLogs = append(filteredLogs, logEntry)
		}
	}

	if len(filteredLogs) > 0 {
		output, err := json.MarshalIndent(filteredLogs, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(output)), nil
	}

	return mcp.NewToolResultText("There are no kubelet logs, this might signal a problem in the VM boot process."), nil
}

func (h *handlers) listClusters(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID := request.GetString("project_id", h.c.DefaultProjectID())
	location, _ := request.RequireString("location")
	if location == "" {
		location = "-"
	}

	req := &containerpb.ListClustersRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectID, location),
	}
	resp, err := h.cmClient.ListClusters(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(protojson.Format(resp)), nil
}

func (h *handlers) getCluster(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	location, err := request.RequireString("location")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	req := &containerpb.GetClusterRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectID, location, name),
	}
	resp, err := h.cmClient.GetCluster(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(protojson.Format(resp)), nil
}

func (h *handlers) listOperations(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID := request.GetString("project_id", h.c.DefaultProjectID())
	location, _ := request.RequireString("location")
	if location == "" {
		location = "-"
	}

	req := &containerpb.ListOperationsRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectID, location),
	}
	resp, err := h.cmClient.ListOperations(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	rawFilter, _ := request.RequireString("filter")
	rawClusterFilter, _ := request.RequireString("filterCluster")
	rawNodepoolFilter, _ := request.RequireString("filterNodepool")

	filter := ""
	if rawFilter != "" {
		filter = rawFilter
	} else if rawClusterFilter != "" {
		filter = fmt.Sprintf("clusters/%s", rawClusterFilter)
	} else if rawClusterFilter != "" {
		filter = fmt.Sprintf("nodePools/%s", rawNodepoolFilter)
	}

	fmt.Printf("Filter was %q\n", filter)
	if filter != "" {
		var filteredOps []*containerpb.Operation
		for _, op := range resp.Operations {
			if strings.Contains(op.TargetLink, filter) {
				filteredOps = append(filteredOps, op)
			}
		}
		resp.Operations = filteredOps
	}

	return mcp.NewToolResultText(protojson.Format(resp)), nil
}

func (h *handlers) describeNodePool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	location, err := request.RequireString("location")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	clusterName, err := request.RequireString("cluster_name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	nodePoolName, err := request.RequireString("nodepool_name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	req := &containerpb.GetNodePoolRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/clusters/%s/nodePools/%s", projectID, location, clusterName, nodePoolName),
	}
	resp, err := h.cmClient.GetNodePool(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(protojson.Format(resp)), nil
}
