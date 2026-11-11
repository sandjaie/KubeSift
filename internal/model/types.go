/*
Purpose: Define the identities and safe messages shared by Go services.
Used by: Connection, trust, session and desktop packages.
Responsibilities: Carry explicit connection generations, resource targets, session references and event envelopes.
Important: Never put credential values or raw helper output into a public error or event.
*/
// Package model defines immutable target identities shared by domain services.
package model

import (
	"crypto/rand"
	"encoding/hex"
)

type ConnectionRef struct {
	ID         string `json:"id"`
	Generation uint64 `json:"generation"`
}
type Target struct {
	Connection ConnectionRef `json:"connection"`
	Namespace  string        `json:"namespace"`
	Name       string        `json:"name"`
	Container  string        `json:"container"`
}
type SessionRef struct {
	ID         string        `json:"id"`
	Connection ConnectionRef `json:"connection"`
}
type Event struct {
	Session  SessionRef `json:"session"`
	Sequence uint64     `json:"sequence"`
	Kind     string     `json:"kind"`
	Data     []byte     `json:"data"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string        { return e.Code + ": " + e.Message }
func Fail(code, message string) error { return &Error{code, message} }
func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Problem is the safe, structured failure shared by product snapshots.
type Problem = Error
