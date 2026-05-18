package uadevice

import "testing"

func TestParse_Chrome_macOS_Desktop(t *testing.T) {
	got := Parse("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	want := Info{BrowserName: "Chrome", BrowserVersion: "120", OSName: "macOS 10.15.7", DeviceKind: "desktop"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParse_Safari_iOS_Mobile(t *testing.T) {
	got := Parse("Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
	if got.BrowserName != "Safari" {
		t.Errorf("browser=%q, want Safari", got.BrowserName)
	}
	if got.OSName != "iOS 17.2" {
		t.Errorf("os=%q, want iOS 17.2", got.OSName)
	}
	if got.DeviceKind != "mobile" {
		t.Errorf("device=%q, want mobile", got.DeviceKind)
	}
}

func TestParse_Firefox_Windows(t *testing.T) {
	got := Parse("Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0")
	if got.BrowserName != "Firefox" || got.OSName != "Windows 10/11" || got.DeviceKind != "desktop" {
		t.Errorf("got %+v", got)
	}
}

func TestParse_Edge_Windows(t *testing.T) {
	got := Parse("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0")
	if got.BrowserName != "Edge" {
		t.Errorf("browser=%q, want Edge (must beat Chrome detection)", got.BrowserName)
	}
}

func TestParse_Android(t *testing.T) {
	got := Parse("Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")
	if got.OSName != "Android 14" || got.DeviceKind != "mobile" {
		t.Errorf("got %+v", got)
	}
}

func TestParse_iPad(t *testing.T) {
	got := Parse("Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
	if got.DeviceKind != "tablet" {
		t.Errorf("device=%q, want tablet", got.DeviceKind)
	}
}

func TestParse_Googlebot(t *testing.T) {
	got := Parse("Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	if got.DeviceKind != "bot" || got.BrowserName != "googlebot" {
		t.Errorf("got %+v", got)
	}
}

func TestParse_Empty(t *testing.T) {
	got := Parse("")
	if got != (Info{}) {
		t.Errorf("got %+v, want zero", got)
	}
}
