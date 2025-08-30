package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/baixiaoshi/tikvtool/client"
	"github.com/baixiaoshi/tikvtool/dao"
	"github.com/baixiaoshi/tikvtool/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	configFile string
	endpoints  []string
)

var rootCmd = &cobra.Command{
	Use:   "tikvtool",
	Short: "Interactive TiKV key explorer",
	Long: `A command-line tool for exploring TiKV keys interactively.
Type key prefixes to search and browse your TiKV data in real-time.`,
	RunE: runExplorer,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file (default is $HOME/.tikvtool.json)")
	rootCmd.PersistentFlags().StringSliceVarP(&endpoints, "endpoints", "e", nil, "TiKV PD endpoints (overrides config file)")
}

func runExplorer(cmd *cobra.Command, args []string) error {
	// 加载配置
	config, err := LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// 如果命令行指定了endpoints，使用命令行的
	pdEndpoints := config.PDAddress
	if len(endpoints) > 0 {
		pdEndpoints = endpoints
	}

	if len(pdEndpoints) == 0 {
		return fmt.Errorf("no PD endpoints specified")
	}

	fmt.Printf("Connecting to TiKV PD endpoints: %v\n", pdEndpoints)

	// 创建TiKV客户端
	ctx := context.Background()
	_, err = client.NewRawKvClient(ctx, pdEndpoints, client.WithApiVersionV2())
	if err != nil {
		return fmt.Errorf("failed to create TiKV client: %v", err)
	}

	fmt.Println("Connected to TiKV successfully!")

	// 创建DAO
	kvClient := dao.NewRawKv()

	// 启动交互式界面
	model := ui.InitialModel(ctx, kvClient)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to start UI: %v", err)
	}

	return nil
}
