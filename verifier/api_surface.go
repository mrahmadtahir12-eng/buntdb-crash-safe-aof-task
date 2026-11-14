package verifier

// Compile-time check that the exported surface of the pinned commit is still
// there with the same signatures (V11). If the candidate removed or changed
// any of these, the verifier does not build.

import (
	"io"
	"time"

	"github.com/tidwall/buntdb"
)

var (
	_ func(string) (*buntdb.DB, error)                                                                        = buntdb.Open
	_ func(*buntdb.DB) error                                                                                  = (*buntdb.DB).Close
	_ func(*buntdb.DB, io.Writer) error                                                                       = (*buntdb.DB).Save
	_ func(*buntdb.DB, io.Reader) error                                                                       = (*buntdb.DB).Load
	_ func(*buntdb.DB, string, string, ...func(a, b string) bool) error                                       = (*buntdb.DB).CreateIndex
	_ func(*buntdb.DB, string, string, ...func(a, b string) bool) error                                       = (*buntdb.DB).ReplaceIndex
	_ func(*buntdb.DB, string, string, func(string) (min, max []float64)) error                               = (*buntdb.DB).CreateSpatialIndex
	_ func(*buntdb.DB, string, string, func(string) (min, max []float64)) error                               = (*buntdb.DB).ReplaceSpatialIndex
	_ func(*buntdb.DB, string) error                                                                          = (*buntdb.DB).DropIndex
	_ func(*buntdb.DB) ([]string, error)                                                                      = (*buntdb.DB).Indexes
	_ func(*buntdb.DB, *buntdb.Config) error                                                                  = (*buntdb.DB).ReadConfig
	_ func(*buntdb.DB, buntdb.Config) error                                                                   = (*buntdb.DB).SetConfig
	_ func(*buntdb.DB) error                                                                                  = (*buntdb.DB).Sync
	_ func(*buntdb.DB) error                                                                                  = (*buntdb.DB).Shrink
	_ func(*buntdb.DB, func(*buntdb.Tx) error) error                                                          = (*buntdb.DB).View
	_ func(*buntdb.DB, func(*buntdb.Tx) error) error                                                          = (*buntdb.DB).Update
	_ func(*buntdb.DB, bool) (*buntdb.Tx, error)                                                              = (*buntdb.DB).Begin
	_ func(*buntdb.Tx) error                                                                                  = (*buntdb.Tx).DeleteAll
	_ func(*buntdb.Tx) error                                                                                  = (*buntdb.Tx).Commit
	_ func(*buntdb.Tx) error                                                                                  = (*buntdb.Tx).Rollback
	_ func(*buntdb.Tx, string) (func(a, b string) bool, error)                                                = (*buntdb.Tx).GetLess
	_ func(*buntdb.Tx, string) (func(string) (min, max []float64), error)                                     = (*buntdb.Tx).GetRect
	_ func(*buntdb.Tx, string, string, *buntdb.SetOptions) (string, bool, error)                              = (*buntdb.Tx).Set
	_ func(*buntdb.Tx, string, ...bool) (string, error)                                                       = (*buntdb.Tx).Get
	_ func(*buntdb.Tx, string) (string, error)                                                                = (*buntdb.Tx).Delete
	_ func(*buntdb.Tx, string) (time.Duration, error)                                                         = (*buntdb.Tx).TTL
	_ func(string, string) bool                                                                               = buntdb.Match
	_ func(*buntdb.Tx, string, func(key, value string) bool) error                                            = (*buntdb.Tx).AscendKeys
	_ func(*buntdb.Tx, string, func(key, value string) bool) error                                            = (*buntdb.Tx).DescendKeys
	_ func(*buntdb.Tx, string, func(key, value string) bool) error                                            = (*buntdb.Tx).Ascend
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).AscendGreaterOrEqual
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).AscendLessThan
	_ func(*buntdb.Tx, string, string, string, func(key, value string) bool) error                            = (*buntdb.Tx).AscendRange
	_ func(*buntdb.Tx, string, func(key, value string) bool) error                                            = (*buntdb.Tx).Descend
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).DescendGreaterThan
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).DescendLessOrEqual
	_ func(*buntdb.Tx, string, string, string, func(key, value string) bool) error                            = (*buntdb.Tx).DescendRange
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).AscendEqual
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).DescendEqual
	_ func(*buntdb.Tx, string, string, func(key, value string, dist float64) bool) error                      = (*buntdb.Tx).Nearby
	_ func(*buntdb.Tx, string, string, func(key, value string) bool) error                                    = (*buntdb.Tx).Intersects
	_ func(*buntdb.Tx) (int, error)                                                                           = (*buntdb.Tx).Len
	_ func(*buntdb.Tx, string, string, ...func(a, b string) bool) error                                       = (*buntdb.Tx).CreateIndex
	_ func(*buntdb.Tx, string, string, *buntdb.IndexOptions, ...func(a, b string) bool) error                 = (*buntdb.Tx).CreateIndexOptions
	_ func(*buntdb.Tx, string, string, func(string) (min, max []float64)) error                               = (*buntdb.Tx).CreateSpatialIndex
	_ func(*buntdb.Tx, string, string, *buntdb.IndexOptions, func(string) (min, max []float64)) error         = (*buntdb.Tx).CreateSpatialIndexOptions
	_ func(*buntdb.Tx, string) error                                                                          = (*buntdb.Tx).DropIndex
	_ func(*buntdb.Tx) ([]string, error)                                                                      = (*buntdb.Tx).Indexes
	_ func([]float64, []float64) string                                                                       = buntdb.Rect
	_ func(...float64) string                                                                                 = buntdb.Point
	_ func(string) (min, max []float64)                                                                       = buntdb.IndexRect
	_ func(string, string) bool                                                                               = buntdb.IndexString
	_ func(string, string) bool                                                                               = buntdb.IndexBinary
	_ func(string, string) bool                                                                               = buntdb.IndexInt
	_ func(string, string) bool                                                                               = buntdb.IndexUint
	_ func(string, string) bool                                                                               = buntdb.IndexFloat
	_ func(string) func(a, b string) bool                                                                     = buntdb.IndexJSON
	_ func(string) func(a, b string) bool                                                                     = buntdb.IndexJSONCaseSensitive
	_ func(func(a, b string) bool) func(a, b string) bool                                                     = buntdb.Desc

	_ error = buntdb.ErrTxNotWritable
	_ error = buntdb.ErrTxClosed
	_ error = buntdb.ErrNotFound
	_ error = buntdb.ErrInvalid
	_ error = buntdb.ErrDatabaseClosed
	_ error = buntdb.ErrIndexExists
	_ error = buntdb.ErrInvalidOperation
	_ error = buntdb.ErrInvalidSyncPolicy
	_ error = buntdb.ErrShrinkInProcess
	_ error = buntdb.ErrPersistenceActive
	_ error = buntdb.ErrTxIterating

	_ buntdb.SyncPolicy = buntdb.Never
	_ buntdb.SyncPolicy = buntdb.EverySecond
	_ buntdb.SyncPolicy = buntdb.Always

	_ = buntdb.Config{
		SyncPolicy:           buntdb.Always,
		AutoShrinkPercentage: 0,
		AutoShrinkMinSize:    0,
		AutoShrinkDisabled:   false,
		OnExpired:            func([]string) {},
		OnExpiredSync:        func(string, string, *buntdb.Tx) error { return nil },
	}
	_ = buntdb.SetOptions{Expires: true, TTL: time.Second}
	_ = buntdb.IndexOptions{CaseInsensitiveKeyMatching: true}
)
