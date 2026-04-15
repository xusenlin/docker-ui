package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strings"

	"docker-ui/internal/docker"
)

func init() {
	if err := initTemplates(); err != nil {
		panic(err)
	}
}

func isImageID(name string) bool {
	matched, _ := regexp.MatchString(`^[a-f0-9]{64}$|^[a-f0-9]{12}$`, name)
	return matched
}

type ContainerHandler struct {
	docker *docker.Client
}

func NewContainerHandler(dc *docker.Client) *ContainerHandler {
	return &ContainerHandler{docker: dc}
}

func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
	containers, err := h.docker.ListContainers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var sb strings.Builder
	if len(containers) == 0 {
		sb.WriteString(`<p class="empty">No containers found.</p>`)
	} else {
		sb.WriteString(`<div class="card-grid">`)
		for _, c := range containers {
			// Ports (max 5)
			var ports []string
			for _, p := range c.Ports {
				ports = append(ports, fmt.Sprintf(`<span class="port-tag">%s → %s</span>`, escapeHTML(p.HostPort), escapeHTML(p.ContainerPort)))
			}
			portsStr := strings.Join(ports, "")
			if portsStr == "" {
				portsStr = "-"
			} else if len(ports) > 5 {
				portsStr = strings.Join(ports[:5], "") + fmt.Sprintf(`<span class="port-tag">+ %d</span>`, len(ports)-5)
			}

			// Action buttons based on state
			var actionsHTML string
			switch c.State {
			case "running":
				actionsHTML = `<button class="btn btn-sm btn-warning" data-label="Pause" onclick="containerAction('` + c.ID + `','pause',this)">Pause</button><button class="btn btn-sm btn-danger" data-label="Stop" onclick="containerAction('` + c.ID + `','stop',this)">Stop</button>`
			case "paused":
				actionsHTML = `<button class="btn btn-sm btn-success" data-label="Unpause" onclick="containerAction('` + c.ID + `','unpause',this)">Unpause</button><button class="btn btn-sm btn-danger" data-label="Stop" onclick="containerAction('` + c.ID + `','stop',this)">Stop</button>`
			case "exited", "created", "restarting":
				actionsHTML = `<button class="btn btn-sm btn-success" data-label="Start" onclick="containerAction('` + c.ID + `','start',this)">Start</button>`
			default:
				actionsHTML = ""
			}

			fmt.Fprintf(&sb, `<div class="card">
    <div class="card-header"><h3>%s</h3><span class="badge badge-%s">%s</span></div>
    <div class="card-body">
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/></svg>Image</span><span class="value">%s</span></div>
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg>Status</span><span class="value">%s</span></div>
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12H19M19 12l-7-7M19 12l-7 7"/></svg>Ports</span><span class="value">%s</span></div>
        <div class="card-body-row"><span class="label"><svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg>Created</span><span class="value">%s</span></div>
    </div>
    <div class="card-footer">
        <div class="card-actions">
            <button class="btn btn-primary btn-sm" onclick="showContainerDetail('%s')">Details</button>
        </div>
        <div class="card-actions">%s</div>
    </div>
</div>`,
				escapeHTML(c.Name), c.State, c.State,
				escapeHTML(c.Image), escapeHTML(c.Status),
				portsStr,
				c.Created.Format("2006-01-02 15:04:05"),
				c.ID,
				actionsHTML)
		}
		sb.WriteString(`</div>`)
	}

	content := sb.String()

	data := map[string]any{
		"CurrentPage": "containers",
		"Title":       "Containers",
		"Count":       len(containers),
		"Content":     template.HTML(content),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = containersTmpl.Execute(w, data)
}

func isSelfContainer(name string) bool {
	return name == "docker-ui"
}

func (h *ContainerHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing container id", http.StatusBadRequest)
		return
	}

	detail, err := h.docker.GetContainerDetail(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	data := map[string]any{
		"name":           detail.Name,
		"id":             detail.ID,
		"image":          detail.Image,
		"status":         detail.Status,
		"state":          detail.State,
		"cmd":            detail.Cmd,
		"restart_policy": detail.RestartPolicy,
		"memory":         humanBytes(detail.Memory),
		"cpu_quota":      detail.CPUQuota,
		"created":        detail.Created.Format("2006-01-02 15:04:05"),
		"ports":          detail.Ports,
		"networks":       detail.Networks,
		"mounts":         detail.Mounts,
		"env_vars":       detail.EnvVars,
		"is_self":        isSelfContainer(detail.Name),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *ContainerHandler) Action(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := r.PathValue("action")

	var err error
	switch action {
	case "start":
		err = h.docker.StartContainer(r.Context(), id)
	case "stop":
		err = h.docker.StopContainer(r.Context(), id)
	case "pause":
		err = h.docker.PauseContainer(r.Context(), id)
	case "unpause":
		err = h.docker.UnpauseContainer(r.Context(), id)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ContainerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing container id", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sendProgress := func(status string, message string) {
		fmt.Fprintf(w, "event: progress\ndata: {\"status\":\"%s\",\"message\":\"%s\"}\n\n", status, message)
		flusher.Flush()
	}

	sendError := func(err string) {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err)
		flusher.Flush()
	}

	sendProgress("starting", "Getting container details...")
	detail, err := h.docker.GetContainerDetail(r.Context(), id)
	if err != nil {
		sendError(err.Error())
		return
	}

	imageName := detail.Image
	if isImageID(imageName) {
		imageName, err = h.docker.GetImageNameByID(r.Context(), imageName)
		if err != nil {
			sendError(err.Error())
			return
		}
	}

	sendProgress("pulling", "Pulling latest image...")
	upToDate, err := h.docker.PullImage(r.Context(), imageName, func(status string, detail string) {
		sendProgress("pulling", detail)
	})
	if err != nil {
		sendError(fmt.Sprintf("pull image failed: %v", err))
		return
	}
	if upToDate {
		sendProgress("up-to-date", "Image is already up to date, no need to recreate")
		sendProgress("done", id)
		flusher.Flush()
		return
	}
	sendProgress("pulled", "New image pulled successfully")

	sendProgress("config", "Getting container configuration...")
	config, err := h.docker.GetRecreateConfig(r.Context(), id)
	if err != nil {
		sendError(err.Error())
		return
	}

	sendProgress("creating", "Creating new container...")
	newID, err := h.docker.RecreateContainer(r.Context(), config)
	if err != nil {
		sendError(err.Error())
		return
	}

	sendProgress("done", newID)
	flusher.Flush()
}
