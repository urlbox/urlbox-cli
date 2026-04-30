// Package api is the single point of HTTP contact with the Urlbox API.
// It exposes the Client interface (mockable) and the HTTPClient implementation.
// No other package speaks HTTP — render, screenshot, pdf, video (and Phase 5's
// status) all go through Client.
package api

import "context"

// Client is the abstract HTTP client the render-adjacent commands talk to.
// Implementations: HTTPClient (real, in this package) and any test fake.
type Client interface {
	// Render performs a synchronous render call. Returns a Response whose
	// Data carries the API's render response body (renderUrl, size,
	// renderTime, queueTime, ... — camelCase per the wire format).
	Render(ctx context.Context, opts map[string]any) (*Response, error)

	// RenderAsync queues a render and returns immediately with a Response
	// whose Data carries at least renderId, status, and statusUrl.
	RenderAsync(ctx context.Context, opts map[string]any) (*Response, error)

	// Status returns the latest state of an async render. Used by Phase 5's
	// urlbox status command. The Data map contains at least: status (one of
	// created|retrying|succeeded|failed|not-found), and on success renderUrl.
	Status(ctx context.Context, renderID string) (*Response, error)
}

// Response is the CLI's intermediate representation of an API call result.
// HTTPClient pre-builds OK/Command/Summary/Breadcrumbs from the API body and
// HTTP status; the calling command finishes assembly into an output.Envelope
// before printing.
//
// Response models the wrapper around upstream API data; the API's wire body
// (camelCase keys like renderUrl, renderId) lives inside Data.
type Response struct {
	OK          bool           `json:"ok"`
	Command     string         `json:"command,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Breadcrumbs []Breadcrumb   `json:"breadcrumbs,omitempty"`
	Error       string         `json:"error,omitempty"`
	Code        string         `json:"code,omitempty"`
	Hint        string         `json:"hint,omitempty"`
}

// Breadcrumb is a "next step" hint emitted alongside a successful Response.
// The render command typically suggests urlbox dashboard, urlbox render
// <url> --output <path>, etc.
type Breadcrumb struct {
	Action string `json:"action"`
	Cmd    string `json:"cmd"`
}
