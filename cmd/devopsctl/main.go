// devopsctl — main CLI entry point
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"devopsctl/internal/agent"
	"devopsctl/internal/controller"
	"devopsctl/internal/devlang"
	"devopsctl/internal/plan"
	"devopsctl/internal/state"
)

func main() {
	root := &cobra.Command{
		Use:   "devopsctl",
		Short: "Programming-first DevOps execution engine",
	}

	// ── devopsctl apply ───────────────────────────────────────────────────────
	var dryRun bool
	var parallelism int
	var resume bool
	var applyLang string

	applyCmd := &cobra.Command{
		Use:   "apply <plan>",
		Short: "Apply an execution plan to target servers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planPath := args[0]
			var (
				p       *plan.Plan
				rawPlan []byte
				err     error
			)
			if filepath.Ext(planPath) == ".devops" {
				// Compile .devops source to plan IR
				src, readErr := os.ReadFile(planPath)
				if readErr != nil {
					return fmt.Errorf("read source: %w", readErr)
				}
				var (
					res     *devlang.CompileResult
					compErr error
				)
				switch applyLang {
				case "", "v0.3":
					res, compErr = devlang.CompileFileV0_3(planPath, src)
				case "v0.5":
					res, compErr = devlang.CompileFileV0_5(planPath, src)
				case "v0.4":
					res, compErr = devlang.CompileFileV0_4(planPath, src)
				case "v0.2":
					res, compErr = devlang.CompileFileV0_2(planPath, src)
				case "v0.1":
					res, compErr = devlang.CompileFileV0_1(planPath, src)
				default:
					return fmt.Errorf("unknown language version %q (supported: v0.1, v0.2, v0.3, v0.4, v0.5)", applyLang)
				}
				if compErr != nil {
					return fmt.Errorf("compile .devops: %w", compErr)
				}
				if len(res.Errors) > 0 {
					for _, e := range res.Errors {
						fmt.Fprintln(os.Stderr, "  ✗", e)
					}
					return fmt.Errorf("compile failed")
				}
				p = res.Plan
				rawPlan = res.RawJSON
			} else {
				p, rawPlan, err = plan.Load(planPath)
				if err != nil {
					return fmt.Errorf("load plan: %w", err)
				}
			}
			if errs := plan.Validate(p); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, "  ✗", e)
				}
				return fmt.Errorf("plan validation failed")
			}
			store, err := state.Open()
			if err != nil {
				return fmt.Errorf("open state store: %w", err)
			}
			defer store.Close()
			return controller.Run(p, rawPlan, store, controller.RunOptions{
				DryRun:      dryRun,
				Parallelism: parallelism,
				Resume:      resume,
			})
		},
	}
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show diff without applying changes")
	applyCmd.Flags().IntVar(&parallelism, "parallelism", 10, "Max concurrent node executions")
	applyCmd.Flags().BoolVar(&resume, "resume", false, "Safely resume execution from the previous failure point")
	applyCmd.Flags().StringVar(&applyLang, "lang", "v0.3", "Language version for .devops plans (v0.1, v0.2, v0.3, v0.4, or v0.5)")

	// ── devopsctl reconcile ───────────────────────────────────────────────────
	var recDryRun bool
	var recParallelism int
	var recLang string

	reconcileCmd := &cobra.Command{
		Use:   "reconcile <plan>",
		Short: "Bring reality in sync with this plan, using recorded state as truth",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planPath := args[0]
			var (
				p       *plan.Plan
				rawPlan []byte
				err     error
			)
			if filepath.Ext(planPath) == ".devops" {
				src, readErr := os.ReadFile(planPath)
				if readErr != nil {
					return fmt.Errorf("read source: %w", readErr)
				}
				var (
					res     *devlang.CompileResult
					compErr error
				)
				switch recLang {
				case "", "v0.3":
					res, compErr = devlang.CompileFileV0_3(planPath, src)
				case "v0.5":
					res, compErr = devlang.CompileFileV0_5(planPath, src)
				case "v0.4":
					res, compErr = devlang.CompileFileV0_4(planPath, src)
				case "v0.2":
					res, compErr = devlang.CompileFileV0_2(planPath, src)
				case "v0.1":
					res, compErr = devlang.CompileFileV0_1(planPath, src)
				default:
					return fmt.Errorf("unknown language version %q (supported: v0.1, v0.2, v0.3, v0.4, v0.5)", recLang)
				}
				if compErr != nil {
					return fmt.Errorf("compile .devops: %w", compErr)
				}
				if len(res.Errors) > 0 {
					for _, e := range res.Errors {
						fmt.Fprintln(os.Stderr, "  ✗", e)
					}
					return fmt.Errorf("compile failed")
				}
				p = res.Plan
				rawPlan = res.RawJSON
			} else {
				p, rawPlan, err = plan.Load(planPath)
				if err != nil {
					return fmt.Errorf("load plan: %w", err)
				}
			}
			if errs := plan.Validate(p); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, "  ✗", e)
				}
				return fmt.Errorf("plan validation failed")
			}
			store, err := state.Open()
			if err != nil {
				return fmt.Errorf("open state store: %w", err)
			}
			defer store.Close()
			return controller.Run(p, rawPlan, store, controller.RunOptions{
				DryRun:      recDryRun,
				Parallelism: recParallelism,
				Reconcile:   true,
			})
		},
	}
	reconcileCmd.Flags().BoolVar(&recDryRun, "dry-run", false, "Show diff without applying changes")
	reconcileCmd.Flags().IntVar(&recParallelism, "parallelism", 10, "Max concurrent node executions")
	reconcileCmd.Flags().StringVar(&recLang, "lang", "v0.3", "Language version for .devops plans (v0.1, v0.2, v0.3, v0.4, or v0.5)")

	// ── devopsctl agent ───────────────────────────────────────────────────────
	var agentAddr string

	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the DevOpsCtl agent daemon on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := &agent.Server{Addr: agentAddr}
			return srv.ListenAndServe()
		},
	}
	agentCmd.Flags().StringVar(&agentAddr, "addr", ":7700", "TCP address to listen on")

	// ── devopsctl state list ──────────────────────────────────────────────────
	var stateNode string

	stateCmd := &cobra.Command{
		Use:   "state",
		Short: "Inspect the local state store",
	}
	stateListCmd := &cobra.Command{
		Use:   "list",
		Short: "List executions from the state store",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := state.Open()
			if err != nil {
				return err
			}
			defer store.Close()
			execs, err := store.List(stateNode)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNODE\tTARGET\tSTATUS\tTIMESTAMP")
			for _, e := range execs {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
					e.ID, e.NodeID, e.Target, e.Status,
					e.Timestamp.Format(time.RFC3339))
			}
			return w.Flush()
		},
	}
	stateListCmd.Flags().StringVar(&stateNode, "node", "", "Filter by node ID")
	stateCmd.AddCommand(stateListCmd)

	// ── devopsctl plan ────────────────────────────────────────────────────────
	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "Manage execution plans",
	}
	planHashCmd := &cobra.Command{
		Use:   "hash <plan.json>",
		Short: "Print the SHA-256 fingerprint of a plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rawData, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%x\n", sha256.Sum256(rawData))
			return nil
		},
	}

	var buildOut string
	var buildLang string
	planBuildCmd := &cobra.Command{
		Use:   "build <file.devops>",
		Short: "Compile a .devops file into a plan JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var (
				res     *devlang.CompileResult
				compErr error
			)
			switch buildLang {
			case "", "v0.3":
				res, compErr = devlang.CompileFileV0_3(path, src)
			case "v0.5":
				res, compErr = devlang.CompileFileV0_5(path, src)
			case "v0.4":
				res, compErr = devlang.CompileFileV0_4(path, src)
			case "v0.2":
				res, compErr = devlang.CompileFileV0_2(path, src)
			case "v0.1":
				res, compErr = devlang.CompileFileV0_1(path, src)
			default:
				return fmt.Errorf("unknown language version %q (supported: v0.1, v0.2, v0.3, v0.4, v0.5)", buildLang)
			}
			if compErr != nil {
				return fmt.Errorf("compile .devops: %w", compErr)
			}
			if len(res.Errors) > 0 {
				for _, e := range res.Errors {
					fmt.Fprintln(os.Stderr, "  ✗", e)
				}
				return fmt.Errorf("compile failed")
			}
			if buildOut == "" {
				os.Stdout.Write(res.RawJSON)
				if len(res.RawJSON) == 0 || res.RawJSON[len(res.RawJSON)-1] != '\n' {
					fmt.Println()
				}
				return nil
			}
			return os.WriteFile(buildOut, res.RawJSON, 0644)
		},
	}
	planBuildCmd.Flags().StringVarP(&buildOut, "output", "o", "", "Output file for compiled plan JSON (default stdout)")
	planBuildCmd.Flags().StringVar(&buildLang, "lang", "v0.3", "Language version for .devops files (v0.1, v0.2, v0.3, v0.4, or v0.5)")
	planCmd.AddCommand(planHashCmd, planBuildCmd)

	// ── devopsctl rollback ────────────────────────────────────────────────────
	var rollbackLast bool
	rollbackCmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback the last execution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !rollbackLast {
				return fmt.Errorf("must specify --last")
			}
			store, err := state.Open()
			if err != nil {
				return err
			}
			defer store.Close()

			return controller.RollbackLast(store)
		},
	}
	rollbackCmd.Flags().BoolVar(&rollbackLast, "last", false, "Rollback the most recent execution")

	root.AddCommand(applyCmd, reconcileCmd, agentCmd, stateCmd, planCmd, rollbackCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
