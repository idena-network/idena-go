package api

import (
	"idena-go/protocol"
)

// FlipApi receives FLIPs, or captchas, submitted
type FlipApi struct {
	pm *protocol.ProtocolManager
}

// NewFlipApi creates a new FlipApi instance
func NewFlipApi(pm *protocol.ProtocolManager) *FlipApi {
	return &FlipApi{pm}
}

// SubmitFlip receives an image as hex
func (api *FlipApi) SubmitFlip(hex string) string {
	return hex
}
