package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon/cmd/rpcdaemon/cli"
	"github.com/erigontech/erigon/rpc"
	"github.com/erigontech/erigon/turbo/debug"
	"github.com/erigontech/erigon/turbo/jsonrpc"
	"github.com/spf13/cobra"

	_ "github.com/erigontech/erigon/core/snaptype"        //hack
	_ "github.com/erigontech/erigon/polygon/bor/snaptype" //hack
)

func main() {
	cmd, cfg := cli.RootCommand()
	rootCtx, rootCancel := common.RootContext()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		logger := debug.SetupCobra(cmd, "rpcdaemon")
		db, backend, txPool, mining, stateCache, blockReader, engine, ff, agg, err := cli.RemoteServices(ctx, cfg, logger, rootCancel)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Error("Could not connect to DB", "err", err)
			}
			return nil
		}
		defer db.Close()
		defer engine.Close()

		var seqRPCService *rpc.Client
		var historicalRPCService *rpc.Client

		// Setup sequencer and hsistorical RPC relay services
		if cfg.RollupSequencerHTTP != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			client, err := rpc.DialContext(ctx, cfg.RollupSequencerHTTP, logger)
			cancel()
			if err != nil {
				logger.Error(err.Error())
				return nil
			}
			seqRPCService = client
		}
		if cfg.RollupHistoricalRPC != "" {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.RollupHistoricalRPCTimeout)
			client, err := rpc.DialContext(ctx, cfg.RollupHistoricalRPC, logger)
			cancel()
			if err != nil {
				logger.Error(err.Error())
				return nil
			}
			historicalRPCService = client
		}

		apiList := jsonrpc.APIList(db, backend, txPool, mining, ff, stateCache, blockReader, agg, cfg, engine, seqRPCService, historicalRPCService, logger)
		rpc.PreAllocateRPCMetricLabels(apiList)
		if err := cli.StartRpcServer(ctx, cfg, apiList, logger); err != nil {
			logger.Error(err.Error())
			return nil
		}

		return nil
	}

	if err := cmd.ExecuteContext(rootCtx); err != nil {
		fmt.Printf("ExecuteContext: %v\n", err)
		os.Exit(1)
	}
}
