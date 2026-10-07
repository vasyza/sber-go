package transport

import (
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

func stringPtr(s string) *string { return &s }

func int64Ptr(v int64) *int64 { return &v }

func syntheticBundle() sdkSession.SessionBundle {
	return sdkSession.SessionBundle{APIBase: "https://web-node2.online.sberbank.ru", WebBase: "https://web2.online.sberbank.ru", SchemaVersion: 4,
		Cookies:     []sdkSession.CookieRecord{{Name: "UFS-SESSION", Value: "synthetic-session-secret", Domain: ".online.sberbank.ru", Path: "/", Secure: true, HTTPOnly: true, HostOnly: false, Expires: int64Ptr(4102444800), SameSite: stringPtr("Strict")}},
		Browser:     sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "User-Agent", Value: "synthetic-agent-secret"}}},
		Deviceprint: stringPtr("synthetic-device-secret"), AntifraudDeviceprint: stringPtr("synthetic-antifraud-secret"), CapturedAt: stringPtr("2026-01-01T00:00:00+00:00"),
	}
}
