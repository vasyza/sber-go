// Package sber is a native Go SDK for the unofficial Sber online-banking
// protocol. It exposes explicit session/authentication factories, typed resource
// APIs, exact decimal amounts, and opt-in financial mutation workflows.
//
// Implementations are layered under internal/session, internal/transport,
// internal/auth and internal/bank; public type aliases preserve the root import.
// Optional browser bootstrap, MCP stdio and rental reconciliation are separate
// packages. Constructors read only caller-selected profiles. A parsed profile
// is not proof of authorization or complete bank history.
package sber
