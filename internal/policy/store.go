package policy

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketDailySpend = []byte("daily_spend")
	bucketAudit      = []byte("audit")
)

// AuditEntry records a signed transaction for the audit log.
type AuditEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Action     string    `json:"action"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	AmountSUN  int64     `json:"amount_sun"`
	WalletName string    `json:"wallet"`
	TxID       string    `json:"txid"`
}

// Store provides persistent storage for daily spend tracking and audit logging.
type Store struct {
	db *bolt.DB
}

// NewStore opens or creates a bbolt database at the given path.
func NewStore(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening policy store: %w", err)
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketDailySpend); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(bucketAudit)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("creating policy store buckets: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// dailySpendKey builds the bucket key for a wallet's daily spend.
func dailySpendKey(wallet string, date time.Time) []byte {
	return []byte(fmt.Sprintf("%s/%s", wallet, date.UTC().Format("2006-01-02")))
}

// GetDailySpend returns the total SUN spent by a wallet on a given date.
func (s *Store) GetDailySpend(wallet string, date time.Time) (int64, error) {
	var total int64
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDailySpend)
		v := b.Get(dailySpendKey(wallet, date))
		if v != nil && len(v) == 8 {
			total = int64(binary.BigEndian.Uint64(v))
		}
		return nil
	})
	return total, err
}

// AddDailySpend atomically adds to a wallet's daily spend counter.
func (s *Store) AddDailySpend(wallet string, date time.Time, amountSUN int64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDailySpend)
		key := dailySpendKey(wallet, date)

		var current int64
		if v := b.Get(key); v != nil && len(v) == 8 {
			current = int64(binary.BigEndian.Uint64(v))
		}

		newTotal := current + amountSUN
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(newTotal))
		return b.Put(key, buf[:])
	})
}

// CheckAndReserve atomically checks if adding amountSUN would exceed limitSUN,
// and if not, reserves it by incrementing the counter. Returns (allowed, currentSpend, error).
// This prevents TOCTOU races between check and record.
func (s *Store) CheckAndReserve(wallet string, date time.Time, amountSUN, limitSUN int64) (bool, int64, error) {
	var current int64
	var allowed bool
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketDailySpend)
		key := dailySpendKey(wallet, date)

		if v := b.Get(key); v != nil && len(v) == 8 {
			current = int64(binary.BigEndian.Uint64(v))
		}

		if current+amountSUN > limitSUN {
			allowed = false
			return nil
		}

		allowed = true
		newTotal := current + amountSUN
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(newTotal))
		return b.Put(key, buf[:])
	})
	return allowed, current, err
}

// RecordAudit appends an entry to the audit log.
func (s *Store) RecordAudit(entry AuditEntry) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAudit)
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		// Key is unix nanoseconds for ordering
		key := fmt.Sprintf("%020d", entry.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}
