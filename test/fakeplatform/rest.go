package fakeplatform

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
)

const (
	paramID                 = "id"
	paramName               = "name"
	sourcePart              = "tgz-code"
	firstCreatedApplication = 690
	createdAt               = "2026-10-15T10:00:00Z"
)

// restRoutes are the deployment API routes that every vcr release calls.
var restRoutes = []route{
	newRoute(http.MethodPost, "/applications", (*Server).createApplication),
	newRoute(http.MethodGet, "/applications", (*Server).listApplications),
	newRoute(http.MethodPatch, "/applications/{id}/keys", (*Server).generateKeys),
	newRoute(http.MethodDelete, "/applications/{id}", (*Server).deleteApplication),
	newRoute(http.MethodPost, "/secrets", (*Server).createSecrets),
	newRoute(http.MethodPatch, "/secrets", (*Server).updateSecrets),
	newRoute(http.MethodDelete, "/secrets/{name}", (*Server).deleteSecret),
	newRoute(http.MethodGet, "/secrets", (*Server).listSecrets),
	newRoute(http.MethodPost, "/projects", (*Server).createProject),
	newRoute(http.MethodPost, "/deployments/validate", (*Server).validateDeployment),
	newRoute(http.MethodPost, "/packages/source", (*Server).uploadSource),
	newRoute(http.MethodPost, "/packages", (*Server).createPackage),
	newRoute(http.MethodPost, "/deployments", (*Server).deployInstance),
	newRoute(http.MethodDelete, "/instances/{id}", (*Server).deleteInstance),
}

type createdApplication struct {
	ApplicationID   string `json:"applicationId"`
	ApplicationName string `json:"applicationName"`
}

type applicationItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type applicationList struct {
	Applications []applicationItem `json:"applications"`
}

type secretsBody struct {
	Secrets []struct {
		Name string `json:"name"`
	} `json:"secrets"`
}

type secretList struct {
	Secrets []string `json:"secrets"`
}

type createdProject struct {
	ProjectID string `json:"projectId"`
}

type validation struct {
	Valid  bool              `json:"valid"`
	Errors []validationIssue `json:"errors,omitempty"`
}

type validationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type uploadedSource struct {
	SourceCodeKey string `json:"sourceCodeKey"`
}

type createdPackage struct {
	PackageID string `json:"packageId"`
}

type deployedInstance struct {
	InstanceID   string   `json:"instanceId"`
	ServiceName  string   `json:"serviceName"`
	DeploymentID string   `json:"deploymentId"`
	HostURLs     []string `json:"hostUrls"`
}

func (s *Server) createApplication(c *call) {
	var req struct {
		Name string `json:"name"`
	}
	if !c.decode(&req) {
		return
	}
	var out createdApplication
	s.locked(func(w *world) {
		app := Application{ID: fixtureID(firstCreatedApplication + w.created), Name: req.Name}
		w.created++
		w.f.Applications = append(w.f.Applications, app)
		out = createdApplication{ApplicationID: app.ID, ApplicationName: app.Name}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) listApplications(c *call) {
	pattern := strings.ToLower(c.r.URL.Query().Get("pattern"))
	out := applicationList{Applications: []applicationItem{}}
	s.locked(func(w *world) {
		for _, a := range w.f.Applications {
			if strings.Contains(strings.ToLower(a.Name), pattern) {
				out.Applications = append(out.Applications, applicationItem(a))
			}
		}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) generateKeys(c *call) {
	id, ok := c.param(paramID)
	s.locked(func(w *world) {
		ok = ok && slices.ContainsFunc(w.f.Applications, func(a Application) bool { return a.ID == id })
	})
	answerEmpty(c, ok, codeApplicationNotFound, msgApplicationNotFound)
}

func (s *Server) deleteApplication(c *call) {
	id, ok := c.param(paramID)
	s.locked(func(w *world) {
		var dropped bool
		w.f.Applications, dropped = without(w.f.Applications, func(a Application) bool { return ok && a.ID == id })
		ok = dropped
	})
	answerEmpty(c, ok, codeApplicationNotFound, msgApplicationNotFound)
}

// answerEmpty answers {} when found, else the 404 envelope with code and message.
func answerEmpty(c *call, found bool, code int, message string) {
	if !found {
		writeError(c.rw, http.StatusNotFound, code, message)
		return
	}
	writeJSON(c.rw, http.StatusOK, empty{})
}

func (s *Server) createSecrets(c *call) {
	var req secretsBody
	if !c.decode(&req) {
		return
	}
	conflict := false
	s.locked(func(w *world) {
		for _, sec := range req.Secrets {
			conflict = conflict || slices.Contains(w.f.Secrets, sec.Name)
		}
		for _, sec := range req.Secrets {
			if !conflict {
				w.f.Secrets = append(w.f.Secrets, sec.Name)
			}
		}
	})
	if conflict {
		writeError(c.rw, http.StatusConflict, codeSecretExists, msgSecretExists)
		return
	}
	writeJSON(c.rw, http.StatusOK, empty{})
}

func (s *Server) updateSecrets(c *call) {
	var req secretsBody
	if !c.decode(&req) {
		return
	}
	found := true
	s.locked(func(w *world) {
		for _, sec := range req.Secrets {
			found = found && slices.Contains(w.f.Secrets, sec.Name)
		}
	})
	answerEmpty(c, found, codeSecretNotFound, msgSecretNotFound)
}

// deleteSecret answers {} whether or not the secret exists, as the platform does.
func (s *Server) deleteSecret(c *call) {
	if name, ok := c.param(paramName); ok {
		s.locked(func(w *world) {
			w.f.Secrets, _ = without(w.f.Secrets, func(v string) bool { return v == name })
		})
	}
	writeJSON(c.rw, http.StatusOK, empty{})
}

func (s *Server) listSecrets(c *call) {
	var out secretList
	s.locked(func(w *world) { out.Secrets = append([]string{}, w.f.Secrets...) })
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) createProject(c *call) {
	var req struct {
		Name string `json:"name"`
	}
	if !c.decode(&req) {
		return
	}
	var out createdProject
	s.locked(func(w *world) {
		out.ProjectID = w.f.Deploy.ProjectID
		if _, exists := w.ownProject(func(p *Project) bool { return p.Name == req.Name }); !exists {
			w.f.Projects = append(w.f.Projects, Project{ID: out.ProjectID, Name: req.Name, CreatedAt: createdAt, UpdatedAt: createdAt})
		}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) validateDeployment(c *call) {
	var out validation
	s.locked(func(w *world) {
		out.Valid = w.f.Deploy.Validate.Valid
		for _, e := range w.f.Deploy.Validate.Errors {
			out.Errors = append(out.Errors, validationIssue(e))
		}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

func (s *Server) uploadSource(c *call) {
	if !hasFormPart(c.r, c.body, sourcePart) {
		writeError(c.rw, http.StatusBadRequest, codeMissingSource, msgMissingSource)
		return
	}
	var out uploadedSource
	s.locked(func(w *world) { out.SourceCodeKey = w.f.Deploy.SourceCodeKey })
	writeJSON(c.rw, http.StatusOK, out)
}

// hasFormPart reports whether body is multipart form data with a part called name.
func hasFormPart(r *http.Request, body []byte, name string) bool {
	media, params, err := mime.ParseMediaType(r.Header.Get(headerContentType))
	if err != nil || !strings.HasPrefix(media, "multipart/") {
		return false
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			return false
		}
		found := part.FormName() == name
		_ = part.Close()
		if found {
			return true
		}
	}
}

func (s *Server) createPackage(c *call) {
	var out createdPackage
	s.locked(func(w *world) { out.PackageID = w.f.Deploy.PackageID })
	writeJSON(c.rw, http.StatusOK, out)
}

// deployInstance answers from the deploy fixture, and the instance joins the
// world, replacing the caller's instance of the same project and name.
func (s *Server) deployInstance(c *call) {
	var req struct {
		ProjectID        string `json:"projectId"`
		APIApplicationID string `json:"apiApplicationId"`
		InstanceName     string `json:"instanceName"`
		Region           string `json:"region"`
	}
	if !c.decode(&req) {
		return
	}
	var out deployedInstance
	s.locked(func(w *world) {
		d := w.f.Deploy
		project, _ := w.ownProject(func(p *Project) bool { return p.ID == req.ProjectID })
		w.upsertInstance(Instance{
			ID: d.InstanceID, Name: req.InstanceName, ServiceName: d.ServiceName, APIApplicationID: req.APIApplicationID,
			ProjectName: project.Name, Region: req.Region, UpdatedAt: w.newestUpdate(),
		})
		out = deployedInstance{
			InstanceID: d.InstanceID, ServiceName: d.ServiceName, DeploymentID: d.DeploymentID,
			HostURLs: append([]string{}, d.HostURLs...),
		}
	})
	writeJSON(c.rw, http.StatusOK, out)
}

// deleteInstance deletes one of the caller's own instances, and refuses one
// that lives in another region than the URL's, as the platform does.
func (s *Server) deleteInstance(c *call) {
	id, ok := c.param(paramID)
	status, code, message := http.StatusNotFound, codeInstanceNotFound, msgInstanceNotFound
	if ok && isUUID(id) {
		s.locked(func(w *world) {
			in := w.instanceByID(id, w.ownedByCaller)
			switch {
			case in == nil:
			case shortAlias(in.Region) != c.region:
				status, code, message = http.StatusConflict, codeOtherRegion, msgOtherRegion
			default:
				in.Deleted = true
				status = http.StatusOK
			}
		})
	}
	if status != http.StatusOK {
		writeError(c.rw, status, code, message)
		return
	}
	writeJSON(c.rw, http.StatusOK, empty{})
}
