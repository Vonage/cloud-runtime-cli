package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

func main() {
	http.HandleFunc("/v0.3/packages/source", uploadTgzHandler)
	http.HandleFunc("/v0.3/packages", createPackageHandler)
	http.HandleFunc("/v0.3/packages/test-package-id/build/watch", watchDeploymentHandler)
	http.HandleFunc("/v0.3/deployments", deployInstanceHandler)
	http.HandleFunc("/releases/latest", getLatestReleaseHandler)

	fmt.Println("Server started on port 80")
	log.Fatal(http.ListenAndServe(":80", nil))
}

type UploadResponse struct {
	SourceCodeKey string `json:"sourceCodeKey"`
}

func uploadTgzHandler(w http.ResponseWriter, _ *http.Request) {
	mockResponse := UploadResponse{SourceCodeKey: "test-key"}
	w.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(mockResponse)
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(jsonResponse); err != nil {
		fmt.Println("Error writing response")
		return
	}
}

type CreatePackageResponse struct {
	PackageID string `json:"packageId"`
}

func createPackageHandler(w http.ResponseWriter, _ *http.Request) {
	mockResponse := CreatePackageResponse{PackageID: "test-package-id"}
	w.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(mockResponse)
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(jsonResponse); err != nil {
		fmt.Println("Error writing response")
		return
	}
}

type DeployInstanceResponse struct {
	InstanceID   string   `json:"instanceId"`
	ServiceName  string   `json:"serviceName"`
	DeploymentID string   `json:"deploymentId"`
	HostURLs     []string `json:"hostUrls"`
}

func deployInstanceHandler(w http.ResponseWriter, _ *http.Request) {
	mockResponse := DeployInstanceResponse{InstanceID: "test-instance-id", ServiceName: "test-service-name", DeploymentID: "test-deployment-id", HostURLs: []string{"test-host-url"}}
	w.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(mockResponse)
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(jsonResponse); err != nil {
		fmt.Println("Error writing response")
		return
	}
}

func watchDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection")
		return
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"status": "completed"}`)); err != nil {
		fmt.Println("Error writing message")
		return
	}
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func getLatestReleaseHandler(w http.ResponseWriter, _ *http.Request) {
	mockResponse := Release{TagName: "v0.0.1"}
	w.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(mockResponse)
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(jsonResponse); err != nil {
		fmt.Println("Error writing response")
		return
	}
}
