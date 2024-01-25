package debug

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"
)

func startDebugProxyServer(appName, localAppHost, hostAddress, websocketServerURL string, proxyWebsocketServerURL string, port int, done <-chan struct{}) error {
	connClient := NewDebuggerConnectionClient(websocketServerURL, proxyWebsocketServerURL, localAppHost)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		provider := getProviderQueryParam(r)
		hostPath := getPathQueryParam(r)
		providerURL := fmt.Sprintf("http://%s%s", provider, path.Join("/", hostPath))
		respStream := connClient.sendRemoteRequest(remoteCommand{
			FAASFunction: provider,
			URL:          providerURL,
			Method:       r.Method,
			Payload:      data,
		}, r.Header.Clone(), r.URL.Query())
		resp := <-respStream

		for k, v := range resp.Headers {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(resp.Status)
		//nolint
		w.Write(resp.Payload)
	})

	server := http.Server{
		Addr:    fmt.Sprintf(":%v", port),
		Handler: mux,
	}

	errStream := make(chan error, 2)
	go func() {
		if err := connClient.run(); err != nil {
			errStream <- fmt.Errorf("failed to run websocket connection: %w", err)
		}
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil {
			errStream <- err
		}
	}()

	logIntroMessage(appName, hostAddress)

	select {
	case err := <-errStream:
		return err
	case <-done:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func getProviderQueryParam(r *http.Request) string {
	// TODO: deprecate "func"
	provider := r.URL.Query().Get("func")
	if v := r.URL.Query().Get("x-neru-debug-provider"); v != "" {
		provider = v
	}
	return provider
}

func getPathQueryParam(r *http.Request) string {
	p := r.URL.Path
	if v := r.URL.Query().Get("path"); v != "" {
		p = v
	}
	if v := r.URL.Query().Get("x-neru-debug-path"); v != "" {
		p = v
	}
	return p
}
