package main

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStoreSecretFromPromptDoesNotPrintSecretValue(t *testing.T) {
	const secretValue = "fake-vercel-token-for-output-test"

	originalStoreSecret := storeSecret
	storeSecret = func(identity, name, value string) error {
		if identity != "client-a" {
			t.Fatalf("identity = %q, want client-a", identity)
		}
		if name != "vercel-token" {
			t.Fatalf("name = %q, want vercel-token", name)
		}
		if value != secretValue {
			t.Fatalf("stored value = %q, want fake secret", value)
		}
		return nil
	}
	t.Cleanup(func() { storeSecret = originalStoreSecret })

	output := captureStdout(t, func() {
		err := storeSecretFromPrompt(
			bufio.NewReader(strings.NewReader(secretValue+"\n")),
			"client-a",
			"vercel-token",
		)
		if err != nil {
			t.Fatalf("storeSecretFromPrompt() error = %v", err)
		}
	})

	if strings.Contains(output, secretValue) {
		t.Fatalf("secret value leaked to stdout: %q", output)
	}
	if !strings.Contains(output, "Stored vercel-token for client-a") {
		t.Fatalf("success output missing, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe

	fn()

	if err := writePipe.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = originalStdout

	data, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatal(err)
	}
	return string(data)
}
