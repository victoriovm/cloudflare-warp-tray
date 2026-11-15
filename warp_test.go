package main

import (
	"testing"
	"unsafe"
)

func TestParseStatus(t *testing.T) {
	for _, test := range []struct {
		name, output string
		want         connectionState
	}{
		{"connected", "Status update: Connected\r\n", connected},
		{"disconnected", "Status update: Disconnected\nReason: Settings Changed", disconnected},
		{"connecting", "Status update: Connecting", connecting},
		{"reconnecting", "Status update: Reconnecting", connecting},
		{"disconnecting", "Status update: Disconnecting", connecting},
		{"whitespace", "  Status update: Connected  ", connected},
		{"error", "Unable to connect to daemon", unknown},
		{"misleading reason", "Status update: Disconnected\nReason: Connected", disconnected},
		{"unknown", "Status update: Something new", unknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := parseStatus(test.output); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestSetText(t *testing.T) {
	var dst [8]uint16
	setText(dst[:], "a very long tooltip")
	if dst[7] != 0 {
		t.Fatal("tooltip must be null terminated")
	}
	setText(dst[:], "ok")
	if dst[0] != 'o' || dst[1] != 'k' || dst[2] != 0 || dst[3] != 0 {
		t.Fatal("old text was not cleared")
	}
}

func TestWindowsStructSizes(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("64-bit ABI assertions")
	}
	for _, test := range []struct {
		name      string
		got, want uintptr
	}{
		{"NOTIFYICONDATAW", unsafe.Sizeof(notifyIconData{}), 976},
		{"WNDCLASSEXW", unsafe.Sizeof(windowClass{}), 80},
		{"MSG", unsafe.Sizeof(message{}), 48},
		{"ICONINFO", unsafe.Sizeof(iconInfo{}), 32},
	} {
		if test.got != test.want {
			t.Errorf("%s: got %d, want %d", test.name, test.got, test.want)
		}
	}
}

func TestMakeIcon(t *testing.T) {
	for _, state := range []connectionState{unknown, disconnected, connected} {
		icon, err := makeIcon(24, state)
		if err != nil {
			t.Fatal(err)
		}
		if icon == 0 {
			t.Fatal("empty icon")
		}
		user32.NewProc("DestroyIcon").Call(icon)
	}
}
