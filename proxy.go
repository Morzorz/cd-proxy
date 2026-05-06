package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *ProxyState) HandleMessages(w http.ResponseWriter, r *http.Request) {
	const maxBodySize = 32 * 1024 * 1024 // 32MB

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		writeProxyError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}

	var bodyMap map[string]any
	if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
		writeProxyError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}

	modelName, ok := bodyMap["model"].(string)
	if !ok || modelName == "" {
		writeProxyError(w, http.StatusBadRequest, "invalid_request_error",
			"missing or invalid 'model' field in request body")
		return
	}

	upstream, upstreamBody, err := s.resolveUpstream(modelName, bodyBytes, bodyMap)
	if err != nil {
		writeProxyError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	isStream, _ := bodyMap["stream"].(bool)

	resp, err := s.doUpstreamRequest(r, upstream, upstreamBody)
	if err != nil {
		writeProxyError(w, http.StatusBadGateway, "proxy_error", fmt.Sprintf("upstream unreachable: %v", err))
		return
	}
	defer resp.Body.Close()

	if isStream && resp.StatusCode == http.StatusOK {
		relayStream(w, resp)
	} else {
		relayNonStream(w, resp)
	}
}

func (s *ProxyState) resolveUpstream(modelName string, bodyBytes []byte, bodyMap map[string]any) (*UpstreamConfig, []byte, error) {
	modelCfg, ok := s.ModelMap[modelName]
	if ok {
		if modelCfg.Upstream.ModelName != "" && modelCfg.Upstream.ModelName != modelName {
			bodyMap["model"] = modelCfg.Upstream.ModelName
			rewritten, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to rewrite request body")
			}
			return &modelCfg.Upstream, rewritten, nil
		}
		return &modelCfg.Upstream, bodyBytes, nil
	}

	if s.DefaultUpstream != nil {
		upstream := s.DefaultUpstream
		if upstream.ModelName != "" && upstream.ModelName != modelName {
			bodyMap["model"] = upstream.ModelName
			rewritten, err := json.Marshal(bodyMap)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to rewrite request body")
			}
			return upstream, rewritten, nil
		}
		return upstream, bodyBytes, nil
	}

	return nil, nil, fmt.Errorf("unknown model %q: available models: [%s]", modelName, strings.Join(s.modelNames(), ", "))
}

func (s *ProxyState) doUpstreamRequest(r *http.Request, upstream *UpstreamConfig, body []byte) (*http.Response, error) {
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", upstream.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("x-api-key", upstream.APIKey)
	if v := r.Header.Get("anthropic-version"); v != "" {
		upstreamReq.Header.Set("anthropic-version", v)
	}
	if v := r.Header.Get("anthropic-beta"); v != "" {
		upstreamReq.Header.Set("anthropic-beta", v)
	}

	return s.HTTPClient.Do(upstreamReq)
}

func (s *ProxyState) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func writeProxyError(w http.ResponseWriter, statusCode int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}
