// bridge.go implements the hidden _coordinator-bridge command, which acts
// as an MCP stdio-to-HTTP bridge for the coordinator server.
package cli

import (
	"github.com/berth-dev/berth/internal/coordinator"
	"github.com/spf13/cobra"
)

var bridgeAddr string
var bridgeBeadID string

var bridgeCmd = &cobra.Command{
	Use:    "_coordinator-bridge",
	Hidden: true,
	Short:  "MCP stdio bridge to coordinator HTTP server (internal use only)",
	RunE:   runBridge,
}

func init() {
	bridgeCmd.Flags().StringVar(&bridgeAddr, "addr", "", "Coordinator server address (host:port)")
	bridgeCmd.Flags().StringVar(&bridgeBeadID, "bead-id", "", "Bead ID for this agent")
	_ = bridgeCmd.MarkFlagRequired("addr")
	_ = bridgeCmd.MarkFlagRequired("bead-id")
}

func runBridge(cmd *cobra.Command, args []string) error {
	return coordinator.RunBridge(bridgeAddr, bridgeBeadID)
}
