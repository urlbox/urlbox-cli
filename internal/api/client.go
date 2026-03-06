package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/urlbox/cli/internal/version"
)

type Client struct {
	APIHost    string
	APISecret  string
	HTTPClient *http.Client
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Field      string
	Body       map[string]interface{}
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("api request failed with status %d", e.StatusCode)
}

func New(apiHost string, apiSecret string) *Client {
	return &Client{
		APIHost:   strings.TrimRight(apiHost, "/"),
		APISecret: apiSecret,
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (c *Client) GetJSON(path string, out interface{}) error {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	return c.doJSON(req, out)
}

func (c *Client) PostJSON(path string, body interface{}, out interface{}) error {
	req, err := c.newJSONRequest(http.MethodPost, path, body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	return c.doJSON(req, out)
}

func (c *Client) PutJSON(path string, body interface{}, out interface{}) error {
	req, err := c.newJSONRequest(http.MethodPut, path, body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	return c.doJSON(req, out)
}

func (c *Client) DeleteJSON(path string, out interface{}) error {
	req, err := c.newRequest(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	return c.doJSON(req, out)
}

func (c *Client) Download(url string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", &APIError{StatusCode: resp.StatusCode, Message: resp.Status}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	return body, resp.Header.Get("Content-Type"), nil
}

func (c *Client) doJSON(req *http.Request, out interface{}) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, body)
	}

	if out == nil || len(body) == 0 {
		return nil
	}

	return json.Unmarshal(body, out)
}

func parseAPIError(statusCode int, body []byte) error {
	if len(body) == 0 {
		return &APIError{StatusCode: statusCode}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return &APIError{StatusCode: statusCode, Message: string(body)}
	}

	apiErr := &APIError{
		StatusCode: statusCode,
		Body:       parsed,
	}

	if errorBody, ok := parsed["error"].(map[string]interface{}); ok {
		if code, ok := errorBody["code"].(string); ok {
			apiErr.Code = code
		}
		if message, ok := errorBody["message"].(string); ok {
			apiErr.Message = message
		}
		if field, ok := errorBody["field"].(string); ok {
			apiErr.Field = field
		}
	} else if message, ok := parsed["error"].(string); ok {
		apiErr.Message = message
	}

	if apiErr.Message == "" {
		if message, ok := parsed["message"].(string); ok {
			apiErr.Message = message
		}
	}

	return apiErr
}

func (c *Client) newJSONRequest(method string, path string, body interface{}) (*http.Request, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest(method, path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *Client) newRequest(method string, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.APIHost+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent())
	if c.APISecret != "" {
		req.Header.Set("Authorization", "Bearer "+c.APISecret)
	}

	return req, nil
}

func (c *Client) userAgent() string {
	return fmt.Sprintf("urlbox-cli/%s", version.Version)
}
