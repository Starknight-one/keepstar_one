// Package geoip wraps the MaxMind GeoLite2 City reader and exposes a
// single Lookup(ip) → {Country, City} surface. The wrapper is intentionally
// nil-safe: callers MAY pass a nil *Reader (e.g. when MAXMIND_DB_PATH env
// var is unset or the file is missing) and get empty Result + nil error
// back, so geo enrichment degrades silently rather than blocking session
// create.
//
// .mmdb file is NOT shipped with the binary. Operators download GeoLite2-City
// from MaxMind (free license required) and point MAXMIND_DB_PATH at it. The
// file is loaded once at startup and held in memory (~70 MB).
package geoip

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/oschwald/geoip2-golang"
)

// Result is the subset of city-DB fields we surface to Settings → Sessions.
type Result struct {
	Country string // ISO 3166-1 alpha-2, e.g. "US"
	City    string // English name; .Names["ru"] is also available if needed
}

// Reader is a nil-safe wrapper around geoip2.Reader. A nil receiver returns
// empty Result + nil error from Lookup, so call sites don't need to branch.
type Reader struct {
	db *geoip2.Reader
}

// Open loads the GeoLite2 City database at the given path. Returns
// (nil, nil) when path is empty or the file is missing — that's the
// expected "geo not configured" state, not an error.
func Open(path string) (*Reader, error) {
	if path == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat geolite2 db: %w", err)
	}
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open geolite2 db: %w", err)
	}
	return &Reader{db: db}, nil
}

// Lookup returns geo info for the given IPv4/IPv6 string. Bad input or a
// nil receiver → zero Result + nil error.
func (r *Reader) Lookup(ipStr string) (Result, error) {
	if r == nil || r.db == nil {
		return Result{}, nil
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Result{}, nil
	}
	rec, err := r.db.City(ip)
	if err != nil {
		return Result{}, fmt.Errorf("geoip city lookup: %w", err)
	}
	return Result{
		Country: rec.Country.IsoCode,
		City:    rec.City.Names["en"],
	}, nil
}

// Close releases the underlying file handle. Safe on nil.
func (r *Reader) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}
