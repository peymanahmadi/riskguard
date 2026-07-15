package riskguard

import "time"

// Transaction represents a single payment transaction submitted for risk
// evaluation. Amount is expressed in minor currency units (e.g. cents) to
// avoid floating point money bugs.
type Transaction struct {
	ID         string
	EntityID   string // the account/customer initiating the transaction
	MerchantID string

	AmountMinor int64
	Currency    string

	IP       string
	DeviceID string

	// Country is the ISO 3166-1 alpha-2 country code the transaction is
	// believed to originate from (e.g. resolved from IP geolocation).
	Country string
	Lat     float64
	Lon     float64

	PaymentMethod string
	CreatedAt     time.Time

	// Metadata carries arbitrary caller-supplied context (e.g. session id,
	// user agent) that custom rules may inspect.
	Metadata map[string]string
}

// Profile holds the known-good history for an entity (customer/account),
// used by rules such as new-device or geo-velocity detection.
type Profile struct {
	EntityID        string
	KnownDeviceIDs  []string
	HomeCountry     string
	LastCountry     string
	LastLat         float64
	LastLon         float64
	LastSeenAt      time.Time
	AverageAmount   int64
	TransactionsSum int64
	TransactionsCnt int64
}

// KnowsDevice reports whether the given device id has been seen before for
// this entity
func (p Profile) KnowsDevice(deviceID string) bool {
	for _, d := range p.KnownDeviceIDs {
		if d == deviceID {
			return true
		}
	}
	return false
}
