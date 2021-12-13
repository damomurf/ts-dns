package main

import "time"

// Tailscale API documented at https://github.com/tailscale/tailscale/blob/main/api.md#tailnet-devices-get

type Tailnet struct {
	Devices []Device `json:"devices"`
}

type Device struct {
	Addresses                 []string `json:"addresses"`
	Authorized                bool     `json:"authorized"`
	BlocksIncomingConnections bool
	ClientVersion             string
	Expires                   time.Time
	Hostname                  string
	Name                      string
	ID                        string
	External                  bool `json:"isExternal"`
	KeyExpiryDisabled         bool
	LastSeen                  time.Time
	OS                        string
	UpdateAvailable           bool
	User                      string
	// This can be empty in responses and causes issues for JSON parsing:
	//Created                   *time.Time `json:"created,omitEmpty"`
}

const (
	DeviceURL = "https://api.tailscale.com/api/v2/tailnet/%s/devices"
)
