// Public authentication API; implementation lives in internal/auth.
package sber

import (
	sdkAuth "github.com/vasyza/sber-sdk/internal/auth"
)

type AuthTransportFactory = sdkAuth.AuthTransportFactory
type AuthOptions = sdkAuth.AuthOptions
type PINAuth = sdkAuth.PINAuth

func NewPINAuth(bundle SessionBundle, o AuthOptions) (*PINAuth, error) {
	return sdkAuth.NewPINAuth(bundle, o)
}

type PINProvider = sdkAuth.PINProvider
type AuthStage = sdkAuth.AuthStage

const AuthStageBootstrap = sdkAuth.AuthStageBootstrap
const AuthStageConfigured = sdkAuth.AuthStageConfigured
const AuthStageOTP = sdkAuth.AuthStageOTP
const AuthStageQR = sdkAuth.AuthStageQR
const AuthStagePINEnrollment = sdkAuth.AuthStagePINEnrollment
const AuthStageAuthenticated = sdkAuth.AuthStageAuthenticated
const AuthStageClosed = sdkAuth.AuthStageClosed

// GenerateFingerprints mints the audited synthetic device identity and its
// percent-encoded antifraud counterpart. It does not spoof observed HTTP/TLS.
func GenerateFingerprints() (Deviceprint, Deviceprint, error) { return sdkAuth.GenerateFingerprints() }

func NewPINAuthFromProfile(path string, o AuthOptions, load ...SessionLoadOptions) (*PINAuth, error) {
	return sdkAuth.NewPINAuthFromProfile(path, o, load...)
}

func NewPrimaryAuthFromProfile(path string, o AuthOptions, load ...SessionLoadOptions) (*PrimaryAuth, error) {
	return sdkAuth.NewPrimaryAuthFromProfile(path, o, load...)
}

type CaptchaAnswer = sdkAuth.CaptchaAnswer
type PrimaryAuth = sdkAuth.PrimaryAuth
type PrimaryLoginOptions = sdkAuth.PrimaryLoginOptions

// NewPrimaryAuth requires an explicit device identity and starts with no cookies.
func NewPrimaryAuth(o AuthOptions) (*PrimaryAuth, error) { return sdkAuth.NewPrimaryAuth(o) }

func NewPrimaryAuthFromBundle(b SessionBundle, o AuthOptions) (*PrimaryAuth, error) {
	return sdkAuth.NewPrimaryAuthFromBundle(b, o)
}

type PhoneAuth = sdkAuth.PhoneAuth
type CardAuth = sdkAuth.CardAuth
type QRAuth = sdkAuth.QRAuth
type QRCode = sdkAuth.QRCode
type QRStatus = sdkAuth.QRStatus

const (
	QRNew         = sdkAuth.QRNew
	QRWaitConfirm = sdkAuth.QRWaitConfirm
	QRConfirmed   = sdkAuth.QRConfirmed
	QRRefused     = sdkAuth.QRRefused
	QRExpired     = sdkAuth.QRExpired
)

func NewPhoneAuth(o AuthOptions) (*PhoneAuth, error) { return sdkAuth.NewPhoneAuth(o) }
func NewCardAuth(o AuthOptions) (*CardAuth, error)   { return sdkAuth.NewCardAuth(o) }
func NewQRAuth(o AuthOptions) (*QRAuth, error)       { return sdkAuth.NewQRAuth(o) }
