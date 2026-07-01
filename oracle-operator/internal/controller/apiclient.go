package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNotFound is returned when the API responds with 404.
var ErrNotFound = errors.New("not found")

// DBRequest is the payload sent to the mock API for create and full-update operations.
type DBRequest struct {
	DbName       string `json:"dbName"`
	Owner        string `json:"owner"`
	Version      string `json:"version"`
	CharacterSet string `json:"characterSet"`
	SizeGB       int32  `json:"sizeGB"`
	ServiceName  string `json:"serviceName,omitempty"`
	PdbName      string `json:"pdbName,omitempty"`
	K8sName      string `json:"k8sName,omitempty"`
	K8sNamespace string `json:"k8sNamespace,omitempty"`
}

// DBResponse is the payload returned by the mock API after create/update.
type DBResponse struct {
	ID      string `json:"id"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

type statusUpdateRequest struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// APIClient calls the mock Oracle middleware API.
type APIClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *APIClient) Create(req DBRequest) (*DBResponse, error) {
	return c.doJSON(http.MethodPost, c.BaseURL+"/databases", req)
}

func (c *APIClient) Update(id string, req DBRequest) (*DBResponse, error) {
	return c.doJSON(http.MethodPut, c.BaseURL+"/databases/"+id, req)
}

func (c *APIClient) Delete(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/databases/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("delete: unexpected status %d", resp.StatusCode)
}

func (c *APIClient) UpdateStatus(id, phase, message string) error {
	_, err := c.doJSON(http.MethodPut, c.BaseURL+"/databases/"+id+"/status",
		statusUpdateRequest{Phase: phase, Message: message})
	return err
}

func (c *APIClient) doJSON(method, url string, body interface{}) (*DBResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%s %s: unexpected status %d", method, url, resp.StatusCode)
	}
	var result DBResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
