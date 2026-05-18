// Package uadevice parses a raw User-Agent header into the handful of
// fields Settings → Sessions actually shows: browser name + version, OS
// name, and a coarse device kind (desktop / mobile / tablet / bot).
//
// We avoid a full UA-parsing library — the matrix is wide but Settings only
// renders a friendly label like "Chrome 120 on macOS · desktop". A short
// regex pass over the common patterns produces good-enough output for that.
// Unknown fields stay empty rather than guessing.
package uadevice

import (
	"regexp"
	"strings"
)

// Info is what we surface to Session row + frontend.
type Info struct {
	BrowserName    string
	BrowserVersion string
	OSName         string
	DeviceKind     string // "desktop" | "mobile" | "tablet" | "bot" | ""
}

// Parse returns best-effort device info. Empty UA → all empty.
func Parse(ua string) Info {
	if ua == "" {
		return Info{}
	}
	low := strings.ToLower(ua)

	// Bots first — they often spoof Chrome UA but include their own token.
	if isBot(low) {
		return Info{DeviceKind: "bot", BrowserName: botName(low)}
	}

	return Info{
		BrowserName:    browserName(ua),
		BrowserVersion: browserVersion(ua),
		OSName:         osName(ua),
		DeviceKind:     deviceKind(low),
	}
}

var (
	reEdge    = regexp.MustCompile(`Edg(?:e|A|iOS)?/(\d+)`)
	reOpera   = regexp.MustCompile(`OPR/(\d+)|Opera/(\d+)`)
	reFirefox = regexp.MustCompile(`Firefox/(\d+)`)
	reChrome  = regexp.MustCompile(`Chrome/(\d+)`)
	reSafari  = regexp.MustCompile(`Version/(\d+)[\d.]*\s+(?:Mobile/\S+\s+)?Safari`)
	reMacOS   = regexp.MustCompile(`Mac OS X (\d+[_\.\d]*)`)
	reWin     = regexp.MustCompile(`Windows NT (\d+\.\d+)`)
	reAndroid = regexp.MustCompile(`Android (\d+(?:\.\d+)?)`)
	reIOS     = regexp.MustCompile(`OS (\d+_\d+(?:_\d+)?) like Mac OS X`)
	reLinux   = regexp.MustCompile(`Linux`)
)

func browserName(ua string) string {
	// Order matters: Edge/Opera embed "Chrome", Chrome embeds "Safari".
	switch {
	case reEdge.MatchString(ua):
		return "Edge"
	case reOpera.MatchString(ua):
		return "Opera"
	case reFirefox.MatchString(ua):
		return "Firefox"
	case reChrome.MatchString(ua):
		return "Chrome"
	case reSafari.MatchString(ua):
		return "Safari"
	}
	return ""
}

func browserVersion(ua string) string {
	for _, re := range []*regexp.Regexp{reEdge, reOpera, reFirefox, reChrome, reSafari} {
		if m := re.FindStringSubmatch(ua); len(m) > 1 {
			for _, g := range m[1:] {
				if g != "" {
					return g
				}
			}
		}
	}
	return ""
}

func osName(ua string) string {
	switch {
	case reAndroid.MatchString(ua):
		return "Android " + reAndroid.FindStringSubmatch(ua)[1]
	case reIOS.MatchString(ua):
		return "iOS " + strings.ReplaceAll(reIOS.FindStringSubmatch(ua)[1], "_", ".")
	case reMacOS.MatchString(ua):
		return "macOS " + strings.ReplaceAll(reMacOS.FindStringSubmatch(ua)[1], "_", ".")
	case reWin.MatchString(ua):
		v := reWin.FindStringSubmatch(ua)[1]
		switch v {
		case "10.0":
			return "Windows 10/11"
		case "6.3":
			return "Windows 8.1"
		case "6.2":
			return "Windows 8"
		case "6.1":
			return "Windows 7"
		}
		return "Windows " + v
	case reLinux.MatchString(ua):
		return "Linux"
	}
	return ""
}

func deviceKind(low string) string {
	switch {
	case strings.Contains(low, "ipad") || strings.Contains(low, "tablet"):
		return "tablet"
	case strings.Contains(low, "iphone") || strings.Contains(low, "mobile") || strings.Contains(low, "android"):
		return "mobile"
	default:
		return "desktop"
	}
}

func isBot(low string) bool {
	for _, m := range []string{"bot", "crawl", "spider", "slurp", "googlebot", "bingbot", "yandexbot", "duckduckbot", "facebookexternal", "pingdom", "monitor", "headlesschrome"} {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

func botName(low string) string {
	for _, n := range []string{"googlebot", "bingbot", "yandexbot", "duckduckbot", "facebookexternalhit", "applebot", "pingdombot", "uptimerobot", "headlesschrome"} {
		if strings.Contains(low, n) {
			return n
		}
	}
	return "bot"
}
