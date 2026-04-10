package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"docker-ui/internal/docker"
)

type ImageHandler struct {
	docker *docker.Client
}

func NewImageHandler(dc *docker.Client) *ImageHandler {
	return &ImageHandler{docker: dc}
}

func shortenImageName(name string) string {
	// e.g. "myregistry.com/myrepo/myimage:v1.2.3" -> truncate if too long
	if len(name) > 40 {
		return "..." + name[len(name)-37:]
	}
	return name
}

func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	images, err := h.docker.ListImages(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var sb strings.Builder
	if len(images) == 0 {
		sb.WriteString(`<p class="empty">No images found.</p>`)
	} else {
		sb.WriteString(`<div class="card-grid">`)
		for _, img := range images {
			// Primary tag: full first tag in header, truncated
			headerTag := ""
			if len(img.Tags) > 0 {
				headerTag = escapeHTML(shortenImageName(img.Tags[0]))
			}

			// Tags row: only show the :tag suffix, not the full name:tag
			var tags []string
			for _, t := range img.Tags {
				parts := strings.Split(t, ":")
				suffix := t
				if len(parts) >= 2 {
					suffix = ":" + parts[len(parts)-1]
				}
				tags = append(tags, fmt.Sprintf(`<span class="tag">%s</span>`, escapeHTML(suffix)))
			}
			tagsHTML := fmt.Sprintf(`<div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"/><line x1="7" y1="7" x2="7.01" y2="7"/></svg>Tags</span><div style="display:flex;flex-wrap:wrap;gap:0.2rem;align-items:baseline">%s</div></div>`, strings.Join(tags, ""))

			fmt.Fprintf(&sb, `<div class="card">
    <div class="card-header"><h3>%s</h3><span class="size-badge">%s</span></div>
    <div class="card-body">
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/></svg>ID</span><span class="value mono">%s</span></div>
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg>Created</span><span class="value">%s</span></div>
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>Size</span><span class="value">%s</span></div>
        %s
    </div>
</div>`,
				headerTag, img.Size,
				img.ID, img.CreatedSince, img.Size,
				tagsHTML)
		}
		sb.WriteString(`</div>`)
	}

	content := sb.String()

	data := map[string]any{
		"CurrentPage": "images",
		"Title":       "Images",
		"Count":       len(images),
		"Content":     template.HTML(content),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = imagesTmpl.Execute(w, data)
}
