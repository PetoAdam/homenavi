package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestBuildPairArgsForThread(t *testing.T) {
	args, networkPath, err := buildPairArgs(config{StorageDir: "/tmp/chip-tool"}, pairRequest{
		Protocol: "matter",
		Mode:     "manual_code",
		Inputs: map[string]any{
			"network_path":               "thread",
			"manual_code":                "34970112332",
			"setup_pin_code":             "20202021",
			"discriminator":              "15",
			"thread_operational_dataset": "hex:abcd",
		},
	}, "123456")
	if err != nil {
		t.Fatalf("buildPairArgs returned error: %v", err)
	}
	want := []string{"pairing", "ble-thread", "123456", "hex:abcd", "20202021", "15", "--storage-directory", "/tmp/chip-tool"}
	if networkPath != "thread" {
		t.Fatalf("unexpected network path: %s", networkPath)
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestBuildPairArgsForThreadQRCode(t *testing.T) {
	args, networkPath, err := buildPairArgs(config{StorageDir: "/tmp/chip-tool"}, pairRequest{
		Protocol: "matter",
		Mode:     "qr_code",
		Inputs: map[string]any{
			"network_path":               "thread",
			"onboarding_payload":         "MT:ABC123456",
			"thread_operational_dataset": "hex:abcd",
		},
	}, "123456")
	if err != nil {
		t.Fatalf("buildPairArgs returned error: %v", err)
	}
	want := []string{"pairing", "code-thread", "123456", "hex:abcd", "MT:ABC123456", "--storage-directory", "/tmp/chip-tool"}
	if networkPath != "thread" {
		t.Fatalf("unexpected network path: %s", networkPath)
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestBuildPairArgsForCodePairing(t *testing.T) {
	args, networkPath, err := buildPairArgs(config{StorageDir: "/tmp/chip-tool", CommissionerName: "alpha"}, pairRequest{
		Protocol: "matter",
		Mode:     "qr_code",
		Inputs: map[string]any{
			"onboarding_payload": "MT:ABC123456",
		},
	}, "777777")
	if err != nil {
		t.Fatalf("buildPairArgs returned error: %v", err)
	}
	want := []string{"pairing", "code", "777777", "MT:ABC123456", "--storage-directory", "/tmp/chip-tool", "--commissioner-name", "alpha"}
	if networkPath != "on_network" {
		t.Fatalf("unexpected network path: %s", networkPath)
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestBuildPairArgsForOnNetwork(t *testing.T) {
	args, networkPath, err := buildPairArgs(config{StorageDir: "/var/lib/matter-commissioner/chip-tool"}, pairRequest{
		Protocol: "matter",
		Mode:     "on_network",
		Inputs: map[string]any{
			"manual_code": "20202021",
		},
	}, "654321")
	if err != nil {
		t.Fatalf("buildPairArgs returned error: %v", err)
	}
	want := []string{"pairing", "onnetwork", "654321", "20202021", "--storage-directory", "/var/lib/matter-commissioner/chip-tool"}
	if networkPath != "on_network" {
		t.Fatalf("unexpected network path: %s", networkPath)
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestBuildPairArgsForOnNetworkLong(t *testing.T) {
	args, networkPath, err := buildPairArgs(config{StorageDir: "/tmp/chip-tool"}, pairRequest{
		Protocol: "matter",
		Mode:     "on_network",
		Inputs: map[string]any{
			"manual_code":   "20202021",
			"discriminator": "3840",
		},
	}, "999999")
	if err != nil {
		t.Fatalf("buildPairArgs returned error: %v", err)
	}
	want := []string{"pairing", "onnetwork-long", "999999", "20202021", "3840", "--storage-directory", "/tmp/chip-tool"}
	if networkPath != "on_network" {
		t.Fatalf("unexpected network path: %s", networkPath)
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestBuildPairArgsNormalizesFormattedManualCode(t *testing.T) {
	args, networkPath, err := buildPairArgs(config{StorageDir: "/tmp/chip-tool"}, pairRequest{
		Protocol: "matter",
		Mode:     "on_network",
		Inputs: map[string]any{
			"manual_code": "3520-203-3941",
		},
	}, "654321")
	if err != nil {
		t.Fatalf("buildPairArgs returned error: %v", err)
	}
	want := []string{"pairing", "onnetwork", "654321", "35202033941", "--storage-directory", "/tmp/chip-tool"}
	if networkPath != "on_network" {
		t.Fatalf("unexpected network path: %s", networkPath)
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
}

func TestDeriveNodeIDDeterministic(t *testing.T) {
	request := pairRequest{Protocol: "matter", Mode: "qr_code", FlowID: "flow-1", Inputs: map[string]any{"onboarding_payload": "MT:ABC", "network_path": "on_network"}}
	first := deriveNodeID(request)
	second := deriveNodeID(request)
	if first == "" || second == "" || first != second {
		t.Fatalf("expected deterministic node id, got %q and %q", first, second)
	}
}

func TestRunPairExecutesChipTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "chip-tool")
	argsFile := filepath.Join(tempDir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"" + argsFile + "\"\nprintf 'commissioned ok'\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake chip-tool: %v", err)
	}
	storageDir := filepath.Join(tempDir, "storage")
	response, err := runPair(config{Binary: binaryPath, StorageDir: storageDir, CommissionerName: "alpha"}, pairRequest{
		Protocol: "matter",
		Mode:     "qr_code",
		Inputs: map[string]any{
			"onboarding_payload": "MT:ABC123",
		},
	}, execCommand)
	if err != nil {
		t.Fatalf("runPair returned error: %v", err)
	}
	if response.Metadata["commissioning_backend"] != "chip-tool" {
		t.Fatalf("unexpected metadata: %#v", response.Metadata)
	}
	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	gotArgs := strings.Fields(string(argsBytes))
	if len(gotArgs) < 6 {
		t.Fatalf("expected captured args, got %#v", gotArgs)
	}
	if gotArgs[0] != "pairing" || gotArgs[1] != "code" {
		t.Fatalf("unexpected chip-tool args: %#v", gotArgs)
	}
	if !strings.Contains(response.Message, "commissioned ok") {
		t.Fatalf("unexpected response message: %#v", response)
	}
	if info, err := os.Stat(storageDir); err != nil || !info.IsDir() {
		t.Fatalf("expected storage dir to exist, err=%v", err)
	}
}

func TestRunPairReportsChipToolFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "chip-tool")
	script := "#!/bin/sh\necho 'pairing failed hard' >&2\nexit 3\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake chip-tool: %v", err)
	}
	_, err := runPair(config{Binary: binaryPath, StorageDir: filepath.Join(tempDir, "storage")}, pairRequest{
		Protocol: "matter",
		Mode:     "qr_code",
		Inputs: map[string]any{
			"onboarding_payload": "MT:ABC123",
		},
	}, execCommand)
	if err == nil || !strings.Contains(err.Error(), "pairing failed hard") {
		t.Fatalf("expected chip-tool failure, got %v", err)
	}
}

func TestRunPairDecodesManualCodeForThreadCommissioning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "chip-tool")
	argsFile := filepath.Join(tempDir, "args.txt")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"payload\" ] && [ \"$2\" = \"parse-setup-payload\" ]; then\n" +
		"  printf '[SPL] Short discriminator: 15\\n[SPL] Passcode: 55610164\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s\\n' \"$@\" > \"" + argsFile + "\"\n" +
		"printf 'commissioned ok'\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake chip-tool: %v", err)
	}
	storageDir := filepath.Join(tempDir, "storage")
	response, err := runPair(config{Binary: binaryPath, StorageDir: storageDir, CommissionerName: "alpha"}, pairRequest{
		Protocol: "matter",
		Mode:     "manual_code",
		Inputs: map[string]any{
			"manual_code":                "3520-203-3941",
			"network_path":               "thread",
			"thread_operational_dataset": "hex:abcd",
		},
	}, execCommand)
	if err != nil {
		t.Fatalf("runPair returned error: %v", err)
	}
	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	gotArgs := strings.Fields(string(argsBytes))
	wantArgs := []string{"pairing", "ble-thread", response.ExternalID, "hex:abcd", "55610164", "15", "--storage-directory", storageDir, "--commissioner-name", "alpha"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected chip-tool args\nwant: %#v\n got: %#v", wantArgs, gotArgs)
	}
	if !strings.Contains(response.Message, "commissioned ok") {
		t.Fatalf("unexpected response message: %#v", response)
	}
}

func TestBuildCommandArgsForTurnOn(t *testing.T) {
	args, state, err := buildCommandArgs(config{StorageDir: "/tmp/chip-tool"}, commandRequest{
		Protocol: "matter",
		DeviceID: "123456",
		Command:  "turn_on",
	}, "123456")
	if err != nil {
		t.Fatalf("buildCommandArgs returned error: %v", err)
	}
	want := []string{"onoff", "on", "123456", "1", "--storage-directory", "/tmp/chip-tool"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
	if state["on"] != true {
		t.Fatalf("expected state on=true, got %#v", state)
	}
}

func TestBuildCommandArgsForSetLevel(t *testing.T) {
	args, state, err := buildCommandArgs(config{StorageDir: "/tmp/chip-tool"}, commandRequest{
		Protocol: "matter",
		DeviceID: "123456",
		Command:  "set_level",
		Args: map[string]any{
			"level": 128,
		},
	}, "123456")
	if err != nil {
		t.Fatalf("buildCommandArgs returned error: %v", err)
	}
	want := []string{"levelcontrol", "move-to-level", "128", "0", "0", "0", "123456", "1", "--storage-directory", "/tmp/chip-tool"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args\nwant: %#v\n got: %#v", want, args)
	}
	if state["level"] != 128 || state["on"] != true {
		t.Fatalf("unexpected state %#v", state)
	}
}

func TestRunCommandExecutesChipTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "chip-tool")
	argsFile := filepath.Join(tempDir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"" + argsFile + "\"\nprintf 'command ok'\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake chip-tool: %v", err)
	}
	response, err := runCommand(config{Binary: binaryPath, StorageDir: filepath.Join(tempDir, "storage")}, commandRequest{
		Protocol: "matter",
		DeviceID: "123456",
		Command:  "turn_off",
	}, execCommand)
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	gotArgs := strings.Fields(string(argsBytes))
	wantArgs := []string{"onoff", "off", "123456", "1", "--storage-directory", filepath.Join(tempDir, "storage")}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected chip-tool args\nwant: %#v\n got: %#v", wantArgs, gotArgs)
	}
	if response.State["on"] != false {
		t.Fatalf("expected on=false state, got %#v", response.State)
	}
	if !strings.Contains(response.Message, "command ok") {
		t.Fatalf("unexpected response message: %#v", response)
	}
}
