package aggregation

import (
	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon/cl/cltypes/solid"
)

//go:generate mockgen -typed=true -destination=./mock_services/aggregation_pool_mock.go -package=mock_services . AggregationPool
type AggregationPool interface {
	// AddAttestation adds a single attestation to the pool.
	AddAttestation(att *solid.Attestation) error
	GetAggregatationByRoot(root common.Hash) *solid.Attestation
}
