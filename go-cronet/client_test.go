package cronet_test

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"go-cronet"
)

func TestClientGetJSON(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/echo"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["method"] != "GET" {
		t.Fatalf("expected GET, got %v", result["method"])
	}
}

func TestClientPostJSON(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	payload := `{"key":"value"}`
	resp, err := client.Post(httpsURL("/echo"), "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["body"] != payload {
		t.Fatalf("expected body %q, got %q", payload, result["body"])
	}
}

func TestClientPostForm(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.PostForm(httpsURL("/echo"), map[string]string{
		"username": "alice",
		"password": "secret",
	})
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Verify content-type header
	headers, _ := result["headers"].(map[string]interface{})
	ct, _ := headers["Content-Type"].(string)
	if !strings.Contains(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("expected form content-type, got %q", ct)
	}
}

func TestClientHead(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Head(httpsURL("/echo"))
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClientStreaming(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/large/1048576"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if n != 1048576 {
		t.Fatalf("expected 1048576 bytes, got %d", n)
	}
}

func TestClientWithAllOptions(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithUserAgent("TestBot/2.0"),
		cronet.WithHTTP2(true),
		cronet.WithBrotli(true),
		cronet.WithZstd(true),
		cronet.WithInsecureSkipVerify(),
		cronet.WithCookies(true),
		cronet.WithTimeout(10*time.Second),
		cronet.WithConnectTimeout(5*time.Second),
		cronet.WithMaxRedirects(5),
		cronet.WithPriority(cronet.PriorityMedium),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/headers"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var headers map[string]string
	json.Unmarshal(body, &headers)

	ua := headers["User-Agent"]
	if !strings.Contains(ua, "TestBot/2.0") {
		t.Fatalf("expected User-Agent containing TestBot/2.0, got %q", ua)
	}
}

func TestClientStdClient(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	stdClient := client.StdClient()
	resp, err := stdClient.Get(httpsURL("/echo"))
	if err != nil {
		t.Fatalf("StdClient.Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClientStatusCodes(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	codes := []int{200, 201, 400, 404, 500}
	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			resp, err := client.Get(httpsURL(fmt.Sprintf("/status/%d", code)))
			if err != nil {
				t.Logf("got error for status %d: %v", code, err)
				return
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body)

			if resp.StatusCode != code {
				t.Fatalf("expected %d, got %d", code, resp.StatusCode)
			}
		})
	}
}
