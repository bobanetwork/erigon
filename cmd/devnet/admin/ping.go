package admin

import (
	"context"

	"github.com/erigontech/erigon/cmd/devnet/devnet"
	"github.com/erigontech/erigon/cmd/devnet/scenarios"
)

func init() {
	scenarios.MustRegisterStepHandlers(
		scenarios.StepHandler(PingErigonRpc),
	)
}

func PingErigonRpc(ctx context.Context) error {
	err := devnet.SelectNode(ctx).PingErigonRpc().Err
	if err != nil {
		devnet.Logger(ctx).Error("FAILURE", "error", err)
	}
	return err
}
