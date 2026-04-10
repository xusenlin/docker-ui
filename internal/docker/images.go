package docker

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"

	"docker-ui/internal/model"
)

func (c *Client) ListImages(ctx context.Context) ([]model.ImageSummary, error) {
	opts := image.ListOptions{All: false}
	raw, err := c.cli.ImageList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	images := make([]model.ImageSummary, 0, len(raw))
	for _, img := range raw {
		tags := make([]string, 0, len(img.RepoTags))
		for _, tag := range img.RepoTags {
			if tag != "<none>:<none>" {
				tags = append(tags, tag)
			}
		}

		images = append(images, model.ImageSummary{
			ID:           img.ID[:12],
			Tags:         tags,
			Size:         humanSize(img.Size),
			SizeRaw:      img.Size,
			Created:      time.Unix(img.Created, 0),
			CreatedSince: timeSince(time.Unix(img.Created, 0)),
		})
	}

	sort.Slice(images, func(i, j int) bool {
		return images[i].SizeRaw > images[j].SizeRaw
	})

	return images, nil
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func timeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hr ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%d years ago", int(d.Hours()/(24*365)))
	}
}

func (c *Client) ContainerExists(ctx context.Context, id string) (bool, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		if c.ID[:12] == id {
			return true, nil
		}
	}
	return false, nil
}
