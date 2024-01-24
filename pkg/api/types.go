package api

import "time"

type Region struct {
	Name              string `json:"name"`
	Alias             string `json:"alias"`
	DeploymentApiURL  string `json:"deployment_api_url"`
	AssetsApiURL      string `json:"assets_api_url"`
	EndpointURLScheme string `json:"endpoint_url_scheme"`
	DebuggerURLScheme string `json:"debugger_url_scheme"`
	HostTemplate      string `json:"host_template"`
}

type Instance struct {
	ID          string `json:"id,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
}

type Runtime struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	APIVersion string `json:"api_version"`
	Comments   string `json:"comments"`
}

type Template struct {
	Key     string `json:"key"`
	Content []byte `json:"content"`
	Size    int    `json:"size"`
}

type Metadata struct {
	Name         string    `json:"name"`
	LastModified time.Time `json:"lastModified"`
	Size         int64     `json:"size"`
}

type Project struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	APIAccountID string `json:"api_account_id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}
