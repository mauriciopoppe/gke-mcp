package cluster

import (
	"context"
	"fmt"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// MIG represents a Managed Instance Group.
type MIG struct {
	Name       string `json:"name"`
	IsStable   bool   `json:"isStable"`
	TargetSize int64  `json:"targetSize"`
}

// ListMigs lists Managed Instance Groups in a project and location.
func ListMigs(ctx context.Context, projectID, location, filter string) ([]MIG, error) {
	computeService, err := compute.NewService(ctx, option.WithUserAgent("gke-mcp-gemini-cli"))
	if err != nil {
		return nil, fmt.Errorf("failed to create compute service: %w", err)
	}

	var migs []MIG
	// This is a simple way to differentiate between a zone and a region.
	// A better way would be to use the locations API.
	if len(location) > 2 && location[len(location)-2] == '-' { // Likely a zone
		req := computeService.InstanceGroupManagers.List(projectID, location)
		if filter != "" {
			req.Filter(filter)
		}
		if err := req.Pages(ctx, func(page *compute.InstanceGroupManagerList) error {
			for _, igm := range page.Items {
				migs = append(migs, MIG{
					Name:       igm.Name,
					IsStable:   igm.Status.IsStable,
					TargetSize: igm.TargetSize,
				})
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("failed to list zonal migs: %w", err)
		}
	} else { // Likely a region
		req := computeService.RegionInstanceGroupManagers.List(projectID, location)
		if filter != "" {
			req.Filter(filter)
		}
		if err := req.Pages(ctx, func(page *compute.RegionInstanceGroupManagerList) error {
			for _, igm := range page.Items {
				migs = append(migs, MIG{
					Name:       igm.Name,
					IsStable:   igm.Status.IsStable,
					TargetSize: igm.TargetSize,
				})
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("failed to list regional migs: %w", err)
		}
	}

	return migs, nil
}