package cmd

import "testing"

func TestBuildStorageRequestForS3(t *testing.T) {
	payload, endpointPath, err := buildStorageRequest(
		stringValue{},
		"aws_s3",
		stringValue{value: "key", set: true},
		stringValue{value: "secret", set: true},
		stringValue{value: "bucket", set: true},
		stringValue{value: "us-east-1", set: true},
		stringValue{},
		stringValue{},
		boolValue{},
		boolValue{},
		stringValue{},
		stringValue{},
		stringValue{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := endpointPath, "/s3/test"; got != want {
		t.Fatalf("unexpected endpoint path: got %q want %q", got, want)
	}
	s3, ok := payload["s3"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected s3 payload")
	}
	if got, want := s3["bucket"], "bucket"; got != want {
		t.Fatalf("unexpected bucket: got %#v want %#v", got, want)
	}
}

func TestBuildStorageRequestForAzure(t *testing.T) {
	payload, endpointPath, err := buildStorageRequest(
		stringValue{},
		"azure",
		stringValue{},
		stringValue{},
		stringValue{},
		stringValue{},
		stringValue{},
		stringValue{},
		boolValue{},
		boolValue{},
		stringValue{value: "acct", set: true},
		stringValue{value: "renders", set: true},
		stringValue{value: "sv=token", set: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := endpointPath, "/azure/test"; got != want {
		t.Fatalf("unexpected endpoint path: got %q want %q", got, want)
	}
	azure, ok := payload["azure"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected azure payload")
	}
	if got, want := azure["accountName"], "acct"; got != want {
		t.Fatalf("unexpected accountName: got %#v want %#v", got, want)
	}
}

func TestMaskSensitiveRedactsMiddle(t *testing.T) {
	if got, want := maskSensitive("abcdefgh12345678", false), "abcd********5678"; got != want {
		t.Fatalf("unexpected masked value: got %q want %q", got, want)
	}
}
