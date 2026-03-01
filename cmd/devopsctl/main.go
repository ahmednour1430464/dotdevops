// devopsctl — main CLI entry point
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"devopsctl/internal/agent"
	"devopsctl/internal/controller"
	"devopsctl/internal/devlang"
	"devopsctl/internal/lsp"
	"devopsctl/internal/pki"
	"devopsctl/internal/plan"
	"devopsctl/internal/secret"
	"devopsctl/internal/state"
)

const version = "0.7.0-dev"

func main() {
	root := &cobra.Command{
		Use:     "devopsctl",
		Short:   "Programming-first DevOps execution engine",
		Version: version,
	}
	var globalOutputFormat string
	root.PersistentFlags().StringVar(&globalOutputFormat, "output", "text", "Output format (text or json)")

	// ── devopsctl apply ───────────────────────────────────────────────────────
	var dryRun bool
	var parallelism int
	var resume bool
	var applyLang string
	var applyTLSCert string
	var applyTLSKey string
	var applyTLSCA string
	var applySecretProvider string
	var applySecretFile string
	var applyTimeout time.Duration

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
				if applyLang != "v0.3" {
					fmt.Fprintf(os.Stderr, "⚠️  WARNING: --lang flag is deprecated. Use 'version = \"%s\"' directive inside %s instead.\n", applyLang, planPath)
				}
				var (
					res     *devlang.CompileResult
					compErr error
				)
				res, compErr = devlang.CompileFileAutoDetect(planPath, src, applyLang)
				if compErr != nil {
					return fmt.Errorf("compile .devops: %w", compErr)
				}
				if len(res.Errors) > 0 {
					reportErrors(res.Errors, globalOutputFormat)
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
				planErrors := make([]error, len(errs))
				for i, e := range errs {
					planErrors[i] = e
				}
				reportErrors(planErrors, globalOutputFormat)
				return fmt.Errorf("plan validation failed")
			}
			store, err := state.Open()
			if err != nil {
				return fmt.Errorf("open state store: %w", err)
			}
			defer store.Close()
			sp, err := secret.NewProvider(applySecretProvider, applySecretFile)
			if err != nil {
				return fmt.Errorf("secret provider: %w", err)
			}
			return controller.Run(p, rawPlan, store, controller.RunOptions{
				DryRun:         dryRun,
				Parallelism:    parallelism,
				Resume:         resume,
				TLSCertPath:    applyTLSCert,
				TLSKeyPath:     applyTLSKey,
				TLSCAPath:      applyTLSCA,
				SecretProvider: sp,
				OutputFormat:   globalOutputFormat,
				Timeout:        applyTimeout,
			})
		},
	}
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show diff without applying changes")
	applyCmd.Flags().IntVar(&parallelism, "parallelism", 10, "Max concurrent node executions")
	applyCmd.Flags().DurationVar(&applyTimeout, "timeout", 0, "Global execution timeout (e.g., '5m', '1h')")
	applyCmd.Flags().BoolVar(&resume, "resume", false, "Safely resume execution from the previous failure point")
	applyCmd.Flags().StringVar(&applyLang, "lang", "v0.3", "Language version for .devops plans (v0.1, v0.2, v0.3, v0.4, v0.5, or v0.6)")
	applyCmd.Flags().StringVar(&applyTLSCert, "tls-cert", "", "Path to client TLS certificate for mTLS")
	applyCmd.Flags().StringVar(&applyTLSKey, "tls-key", "", "Path to client TLS key for mTLS")
	applyCmd.Flags().StringVar(&applyTLSCA, "tls-ca", "", "Path to CA certificate for mTLS")
	applyCmd.Flags().StringVar(&applySecretProvider, "secret-provider", "env", "Secret provider: 'env' (default, reads env vars) or 'file'")
	applyCmd.Flags().StringVar(&applySecretFile, "secret-file", "", "Path to JSON secrets file (required when --secret-provider=file)")

	// ── devopsctl reconcile ───────────────────────────────────────────────────
	var recDryRun bool
	var recParallelism int
	var recLang string
	var recTLSCert string
	var recTLSKey string
	var recTLSCA string
	var recSecretProvider string
	var recSecretFile string
	var recTimeout time.Duration

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
				if recLang != "v0.3" {
					fmt.Fprintf(os.Stderr, "⚠️  WARNING: --lang flag is deprecated. Use 'version = \"%s\"' directive inside %s instead.\n", recLang, planPath)
				}
				var (
					res     *devlang.CompileResult
					compErr error
				)
				res, compErr = devlang.CompileFileAutoDetect(planPath, src, recLang)
				if compErr != nil {
					return fmt.Errorf("compile .devops: %w", compErr)
				}
				if len(res.Errors) > 0 {
					reportErrors(res.Errors, globalOutputFormat)
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
				planErrors := make([]error, len(errs))
				for i, e := range errs {
					planErrors[i] = e
				}
				reportErrors(planErrors, globalOutputFormat)
				return fmt.Errorf("plan validation failed")
			}
			store, err := state.Open()
			if err != nil {
				return fmt.Errorf("open state store: %w", err)
			}
			defer store.Close()
			sp, err := secret.NewProvider(recSecretProvider, recSecretFile)
			if err != nil {
				return fmt.Errorf("secret provider: %w", err)
			}
			return controller.Run(p, rawPlan, store, controller.RunOptions{
				DryRun:         recDryRun,
				Parallelism:    recParallelism,
				Reconcile:      true,
				TLSCertPath:    recTLSCert,
				TLSKeyPath:     recTLSKey,
				TLSCAPath:      recTLSCA,
				SecretProvider: sp,
				OutputFormat:   globalOutputFormat,
				Timeout:        recTimeout,
			})
		},
	}
	reconcileCmd.Flags().BoolVar(&recDryRun, "dry-run", false, "Show diff without applying changes")
	reconcileCmd.Flags().IntVar(&recParallelism, "parallelism", 10, "Max concurrent node executions")
	reconcileCmd.Flags().DurationVar(&recTimeout, "timeout", 0, "Global execution timeout (e.g., '5m', '1h')")
	reconcileCmd.Flags().StringVar(&recLang, "lang", "v0.3", "Language version for .devops plans (v0.1, v0.2, v0.3, v0.4, v0.5, or v0.6)")
	reconcileCmd.Flags().StringVar(&recTLSCert, "tls-cert", "", "Path to client TLS certificate for mTLS")
	reconcileCmd.Flags().StringVar(&recTLSKey, "tls-key", "", "Path to client TLS key for mTLS")
	reconcileCmd.Flags().StringVar(&recTLSCA, "tls-ca", "", "Path to CA certificate for mTLS")
	reconcileCmd.Flags().StringVar(&recSecretProvider, "secret-provider", "env", "Secret provider: 'env' (default, reads env vars) or 'file'")
	reconcileCmd.Flags().StringVar(&recSecretFile, "secret-file", "", "Path to JSON secrets file (required when --secret-provider=file)")

	// ── devopsctl agent ───────────────────────────────────────────────────────
	var agentAddr string
	var agentContextsPath string
	var agentAuditLog string
	var agentTLSCert string
	var agentTLSKey string
	var agentTLSCA string

	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the DevOpsCtl agent daemon on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentContextsPath == "" {
				return fmt.Errorf("--contexts flag is required")
			}

			if agentTLSCert == "" || agentTLSKey == "" {
				// Always print security warning if mTLS is disabled
				fmt.Fprintln(os.Stderr, "⚠️  SECURITY WARNING: Running agent without mTLS enabled")
				fmt.Fprintln(os.Stderr, "   Execute commands from untrusted controllers may occur. Use in isolated networks only.")
			}

			srv := &agent.Server{
				Addr:         agentAddr,
				ContextsPath: agentContextsPath,
				AuditLogPath: agentAuditLog,
				TLSCertPath:  agentTLSCert,
				TLSKeyPath:   agentTLSKey,
				TLSCAPath:    agentTLSCA,
			}
			return srv.ListenAndServe()
		},
	}
	agentCmd.Flags().StringVar(&agentAddr, "addr", ":7700", "TCP address to listen on")
	agentCmd.Flags().StringVar(&agentContextsPath, "contexts", "",
		"Path to execution contexts config file (REQUIRED)")
	agentCmd.Flags().StringVar(&agentAuditLog, "audit-log", "/var/log/devopsctl-audit.log",
		"Path to audit log file")
	agentCmd.Flags().StringVar(&agentTLSCert, "tls-cert", "", "Path to TLS certificate for agent mTLS")
	agentCmd.Flags().StringVar(&agentTLSKey, "tls-key", "", "Path to TLS key for agent mTLS")
	agentCmd.Flags().StringVar(&agentTLSCA, "tls-ca", "", "Path to CA certificate for agent mTLS")

	agentStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check the status of a running DevOpsCtl agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Basic TCP test for now. In real prod, this should use mTLS if configured.
			conn, err := net.DialTimeout("tcp", agentAddr, 3*time.Second)
			if err != nil {
				return fmt.Errorf("agent unreachable at %s: %v", agentAddr, err)
			}
			defer conn.Close()

			enc := json.NewEncoder(conn)
			if err := enc.Encode(map[string]string{"type": "status_req"}); err != nil {
				return fmt.Errorf("send request: %v", err)
			}

			// Read response
			r := bufio.NewReader(conn)
			line, err := r.ReadBytes('\n')
			if err != nil {
				return fmt.Errorf("read response: %v", err)
			}

			var resp map[string]any
			if err := json.Unmarshal(line, &resp); err != nil {
				return fmt.Errorf("parse response: %v", err)
			}

			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}
	agentStatusCmd.Flags().StringVar(&agentAddr, "addr", ":7700", "TCP address of the agent")
	agentCmd.AddCommand(agentStatusCmd)

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
			if globalOutputFormat == "json" {
				b, _ := json.MarshalIndent(execs, "", "  ")
				fmt.Println(string(b))
				return nil
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
			if buildLang != "v0.3" {
				fmt.Fprintf(os.Stderr, "⚠️  WARNING: --lang flag is deprecated. Use 'version = \"%s\"' directive inside %s instead.\n", buildLang, path)
			}
			var (
				res     *devlang.CompileResult
				compErr error
			)
			res, compErr = devlang.CompileFileAutoDetect(path, src, buildLang)
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
	planBuildCmd.Flags().StringVar(&buildLang, "lang", "v0.3", "Language version for .devops files (v0.1, v0.2, v0.3, v0.4, v0.5, or v0.6)")

	planDiffCmd := &cobra.Command{
		Use:   "diff <old.plan> <new.plan>",
		Short: "Show the semantic difference between two plans",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			load := func(p string) (*plan.Plan, error) {
				if filepath.Ext(p) == ".devops" {
					src, err := os.ReadFile(p)
					if err != nil {
						return nil, err
					}
					res, err := devlang.CompileFileAutoDetect(p, src, "v0.8")
					if err != nil {
						return nil, err
					}
					if len(res.Errors) > 0 {
						reportErrors(res.Errors, globalOutputFormat)
						return nil, fmt.Errorf("compile failed for %s", p)
					}
					return res.Plan, nil
				}
				pl, _, err := plan.Load(p)
				return pl, err
			}
			oldPlan, err := load(args[0])
			if err != nil {
				return err
			}
			newPlan, err := load(args[1])
			if err != nil {
				return err
			}

			diff := plan.Diff(oldPlan, newPlan)

			if globalOutputFormat == "json" {
				b, _ := json.MarshalIndent(diff, "", "  ")
				fmt.Println(string(b))
				if diff.HasChanges() {
					os.Exit(1)
				}
				return nil
			}

			if !diff.HasChanges() {
				fmt.Println("No semantic changes.")
				return nil
			}

			for _, n := range diff.Added {
				fmt.Printf("[+] %s\t(Added)\n", n.ID)
			}
			for _, n := range diff.Removed {
				fmt.Printf("[-] %s\t(Removed)\n", n.ID)
			}
			for _, d := range diff.Changed {
				if d.Old.Type != d.New.Type {
					fmt.Printf("[~] %s\t(Changed: type %s → %s)\n", d.New.ID, d.Old.Type, d.New.Type)
				} else {
					fmt.Printf("[~] %s\t(Changed)\n", d.New.ID)
				}
			}
			os.Exit(1)
			return nil
		},
	}

	planCmd.AddCommand(planHashCmd, planBuildCmd, planDiffCmd)

	// ── devopsctl rollback ────────────────────────────────────────────────────
	var rollbackLast bool
	var rollbackConfirm bool
	var rollbackTLSCert string
	var rollbackTLSKey string
	var rollbackTLSCA string
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

			return controller.RollbackLast(store, controller.RunOptions{
				Confirm:      rollbackConfirm,
				TLSCertPath:  rollbackTLSCert,
				TLSKeyPath:   rollbackTLSKey,
				TLSCAPath:    rollbackTLSCA,
				OutputFormat: globalOutputFormat,
			})
		},
	}
	rollbackCmd.Flags().BoolVar(&rollbackLast, "last", false, "Rollback the most recent execution")
	rollbackCmd.Flags().BoolVar(&rollbackConfirm, "confirm", false, "Confirm rollback of effectful nodes lacking rollback_cmd")
	rollbackCmd.Flags().StringVar(&rollbackTLSCert, "tls-cert", "", "Path to client TLS certificate for mTLS")
	rollbackCmd.Flags().StringVar(&rollbackTLSKey, "tls-key", "", "Path to client TLS key for mTLS")
	rollbackCmd.Flags().StringVar(&rollbackTLSCA, "tls-ca", "", "Path to CA certificate for mTLS")

	// ── devopsctl observe ─────────────────────────────────────────────────────
	var obsParallelism int
	var obsLang string
	var obsTLSCert string
	var obsTLSKey string
	var obsTLSCA string
	var obsTimeout time.Duration

	observeCmd := &cobra.Command{
		Use:   "observe <plan>",
		Short: "Observe reality against the plan without making changes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planPath := args[0]
			var (
				p       *plan.Plan
				rawPlan []byte
			)
			if filepath.Ext(planPath) == ".devops" {
				src, err := os.ReadFile(planPath)
				if err != nil {
					return err
				}
				res, err := devlang.CompileFileAutoDetect(planPath, src, obsLang)
				if err != nil {
					return err
				}
				if len(res.Errors) > 0 {
					reportErrors(res.Errors, globalOutputFormat)
					return fmt.Errorf("compile failed")
				}
				p = res.Plan
				rawPlan = res.RawJSON
			} else {
				var err error
				p, rawPlan, err = plan.Load(planPath)
				if err != nil {
					return err
				}
			}
			if errs := plan.Validate(p); len(errs) > 0 {
				planErrors := make([]error, len(errs))
				for i, e := range errs {
					planErrors[i] = e
				}
				reportErrors(planErrors, globalOutputFormat)
				return fmt.Errorf("plan validation failed")
			}
			store, err := state.Open()
			if err != nil {
				return err
			}
			defer store.Close()

			return controller.Run(p, rawPlan, store, controller.RunOptions{
				Parallelism:  obsParallelism,
				Observe:      true,
				TLSCertPath:  obsTLSCert,
				TLSKeyPath:   obsTLSKey,
				TLSCAPath:    obsTLSCA,
				OutputFormat: globalOutputFormat,
				Timeout:      obsTimeout,
			})
		},
	}
	observeCmd.Flags().IntVar(&obsParallelism, "parallelism", 10, "Max concurrent observations")
	observeCmd.Flags().DurationVar(&obsTimeout, "timeout", 0, "Global execution timeout (e.g., '5m', '1h')")
	observeCmd.Flags().StringVar(&obsLang, "lang", "v0.3", "Language version (deprecated, use 'version' directive)")
	observeCmd.Flags().StringVar(&obsTLSCert, "tls-cert", "", "Path to client TLS certificate")
	observeCmd.Flags().StringVar(&obsTLSKey, "tls-key", "", "Path to client TLS key")
	observeCmd.Flags().StringVar(&obsTLSCA, "tls-ca", "", "Path to CA certificate")

	// ── devopsctl pki ─────────────────────────────────────────────────────────
	pkiCmd := &cobra.Command{
		Use:   "pki",
		Short: "PKI tools for bootstrapping mTLS certificates",
	}

	var pkiOutDir string
	var pkiValidYears int
	pkiInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a self-signed CA and controller/agent leaf certificates for mTLS",
		Long: `Generates six files in the output directory:
  ca.crt / ca.key          — self-signed certificate authority
  controller.crt / .key    — certificate pair for the devopsctl CLI (controller)
  agent.crt / .key         — certificate pair for the devopsctl agent daemon

Use --tls-cert, --tls-key, --tls-ca on all devopsctl commands to enable mTLS.

IMPORTANT: This tool is for development and homelab use only.
For production, use certificates from an external CA.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := pki.InitOptions{
				OutputDir: pkiOutDir,
				ValidFor:  time.Duration(pkiValidYears) * 365 * 24 * time.Hour,
			}
			bundle, err := pki.Init(opts)
			if err != nil {
				return fmt.Errorf("pki init: %w", err)
			}

			if globalOutputFormat == "json" {
				b, _ := json.MarshalIndent(bundle, "", "  ")
				fmt.Println(string(b))
				return nil
			}

			fmt.Println("✓ PKI initialised successfully")
			fmt.Println()
			fmt.Printf("  CA certificate : %s\n", bundle.CACert)
			fmt.Printf("  CA private key : %s  (keep secret)\n", bundle.CAKey)
			fmt.Printf("  Controller cert: %s\n", bundle.ControllerCert)
			fmt.Printf("  Controller key : %s  (keep secret)\n", bundle.ControllerKey)
			fmt.Printf("  Agent cert     : %s\n", bundle.AgentCert)
			fmt.Printf("  Agent key      : %s  (keep secret)\n", bundle.AgentKey)
			fmt.Println()
			fmt.Println("Start the agent with mTLS:")
			fmt.Printf("  devopsctl agent --tls-cert %s --tls-key %s --tls-ca %s\n",
				bundle.AgentCert, bundle.AgentKey, bundle.CACert)
			fmt.Println()
			fmt.Println("Run commands with mTLS:")
			fmt.Printf("  devopsctl apply plan.devops --tls-cert %s --tls-key %s --tls-ca %s\n",
				bundle.ControllerCert, bundle.ControllerKey, bundle.CACert)
			return nil
		},
	}
	pkiInitCmd.Flags().StringVar(&pkiOutDir, "out", "./pki", "Directory to write generated certificate and key files")
	pkiInitCmd.Flags().IntVar(&pkiValidYears, "valid-years", 10, "Certificate validity period in years")
	pkiCmd.AddCommand(pkiInitCmd)

	// ── devopsctl lsp ─────────────────────────────────────────────────────────
	var lspStdio bool
	lspCmd := &cobra.Command{
		Use:   "lsp",
		Short: "Start Language Server Protocol (LSP) for .devops files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !lspStdio {
				return fmt.Errorf("only --stdio mode is supported for now")
			}
			return lsp.Serve()
		},
	}
	lspCmd.Flags().BoolVar(&lspStdio, "stdio", false, "Communicate via stdio")

	// ── devopsctl diff (top-level alias for plan diff) ───────────────────────
	var diffLang string
	diffCmd := &cobra.Command{
		Use:   "diff <old.plan> <new.plan>",
		Short: "Show the semantic difference between two plans",
		Long:  "Compile both plans and display which nodes were added, removed, or changed.\nAlias for 'devopsctl plan diff'.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			load := func(p string) (*plan.Plan, error) {
				if filepath.Ext(p) == ".devops" {
					src, err := os.ReadFile(p)
					if err != nil {
						return nil, err
					}
					res, err := devlang.CompileFileAutoDetect(p, src, diffLang)
					if err != nil {
						return nil, err
					}
					if len(res.Errors) > 0 {
						reportErrors(res.Errors, globalOutputFormat)
						return nil, fmt.Errorf("compile failed for %s", p)
					}
					return res.Plan, nil
				}
				pl, _, err := plan.Load(p)
				return pl, err
			}
			oldPlan, err := load(args[0])
			if err != nil {
				return err
			}
			newPlan, err := load(args[1])
			if err != nil {
				return err
			}

			diff := plan.Diff(oldPlan, newPlan)

			if globalOutputFormat == "json" {
				b, _ := json.MarshalIndent(diff, "", "  ")
				fmt.Println(string(b))
				if diff.HasChanges() {
					os.Exit(1)
				}
				return nil
			}

			if !diff.HasChanges() {
				fmt.Println("No semantic changes.")
				return nil
			}

			for _, n := range diff.Added {
				fmt.Printf("[+] %s\t(Added)\n", n.ID)
			}
			for _, n := range diff.Removed {
				fmt.Printf("[-] %s\t(Removed)\n", n.ID)
			}
			for _, d := range diff.Changed {
				if d.Old.Type != d.New.Type {
					fmt.Printf("[~] %s\t(Changed: type %s → %s)\n", d.New.ID, d.Old.Type, d.New.Type)
				} else {
					fmt.Printf("[~] %s\t(Changed)\n", d.New.ID)
				}
			}
			os.Exit(1)
			return nil
		},
	}
	diffCmd.Flags().StringVar(&diffLang, "lang", "v0.8", "Default language version for .devops files without version directive")

	// ── devopsctl validate ────────────────────────────────────────────────────
	var validateLang string
	validateCmd := &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate (compile) a plan without executing it",
		Long:  "Compiles a .devops file (or validates a .json plan) and reports errors.\nExits 0 on success, 1 if there are any errors. No agent connection is made.\nIdeal for pre-commit hooks and CI validation.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			planPath := args[0]
			if filepath.Ext(planPath) == ".devops" {
				src, err := os.ReadFile(planPath)
				if err != nil {
					return fmt.Errorf("read source: %w", err)
				}
				res, err := devlang.CompileFileAutoDetect(planPath, src, validateLang)
				if err != nil {
					return fmt.Errorf("compile: %w", err)
				}
				if len(res.Errors) > 0 {
					reportErrors(res.Errors, globalOutputFormat)
					return fmt.Errorf("%d error(s) found", len(res.Errors))
				}
				if globalOutputFormat == "json" {
					b, _ := json.MarshalIndent(res.Plan, "", "  ")
					fmt.Println(string(b))
				} else {
					fmt.Printf("✓ %s is valid (%d node(s), %d target(s))\n",
						planPath, len(res.Plan.Nodes), len(res.Plan.Targets))
				}
				return nil
			}
			// JSON plan — just load and validate the IR.
			p, _, err := plan.Load(planPath)
			if err != nil {
				return fmt.Errorf("load plan: %w", err)
			}
			if errs := plan.Validate(p); len(errs) > 0 {
				planErrors := make([]error, len(errs))
				for i, e := range errs {
					planErrors[i] = e
				}
				reportErrors(planErrors, globalOutputFormat)
				return fmt.Errorf("%d error(s) found", len(errs))
			}
			if globalOutputFormat == "json" {
				b, _ := json.MarshalIndent(p, "", "  ")
				fmt.Println(string(b))
			} else {
				fmt.Printf("✓ %s is valid (%d node(s), %d target(s))\n",
					planPath, len(p.Nodes), len(p.Targets))
			}
			return nil
		},
	}
	validateCmd.Flags().StringVar(&validateLang, "lang", "v0.8", "Default language version for .devops files without version directive")

	// ── devopsctl init ────────────────────────────────────────────────────────
	var initDir string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new DevOpsCTL project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.MkdirAll(initDir, 0755); err != nil {
				return err
			}

			fmt.Println("Initializing project in", initDir, "...")

			// 1. Write starter plan.devops
			planPath := filepath.Join(initDir, "plan.devops")
			if _, err := os.Stat(planPath); os.IsNotExist(err) {
				planContent := `version = "v1.2"

target "local" {
	address = "127.0.0.1:7700"
}

node "hello" {
	type = process.exec
	targets = [local]
	
	cmd = ["echo", "Hello, DevOpsCTL!"]
}
`
				if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
					return err
				}
				fmt.Println("  Created", planPath)
			}

			// 2. PKI
			pkiDir := filepath.Join(initDir, "pki")
			if _, err := os.Stat(pkiDir); os.IsNotExist(err) {
				if err := os.MkdirAll(pkiDir, 0700); err != nil {
					return err
				}
				opts := pki.InitOptions{
					OutputDir: pkiDir,
					ValidFor:  10 * 365 * 24 * time.Hour,
				}
				_, err := pki.Init(opts)
				if err != nil {
					return err
				}
				fmt.Println("  Created PKI certificates in", pkiDir)
			}

			// 3. contexts.yaml
			ctxPath := filepath.Join(initDir, "contexts.yaml")
			if _, err := os.Stat(ctxPath); os.IsNotExist(err) {
				ctxContent := `contexts:
  - name: default
    purpose: Development context
    trust_level: high
    identity:
      user: $USER
      group: $USER
    privilege:
      allow_escalation: true
    filesystem:
      readable_paths: ["/"]
      writable_paths: ["/tmp", "/var/tmp", "."]
    process:
      denied_executables: []
      resource_limits:
        max_processes: 100
    network:
      allow_network: true
      scope: full
    audit:
      level: standard
      log_stdout: true
      log_stderr: true
`
				if err := os.WriteFile(ctxPath, []byte(ctxContent), 0644); err != nil {
					return err
				}
				fmt.Println("  Created", ctxPath)
			}

			fmt.Println("\n✓ Project initialized successfully!")
			fmt.Println("\nNext steps:")
			fmt.Println("  1. Start the local agent:")
			fmt.Println("     ./devopsctl agent --addr :7700 --contexts contexts.yaml --tls-cert pki/agent.crt --tls-key pki/agent.key --tls-ca pki/ca.crt")
			fmt.Println("  2. Apply the plan:")
			fmt.Println("     ./devopsctl apply plan.devops --tls-cert pki/controller.crt --tls-key pki/controller.key --tls-ca pki/ca.crt")
			
			return nil
		},
	}
	initCmd.Flags().StringVar(&initDir, "dir", ".", "Directory to scaffold the project into")

	root.AddCommand(applyCmd, reconcileCmd, agentCmd, stateCmd, planCmd, rollbackCmd, observeCmd, pkiCmd, lspCmd, diffCmd, validateCmd, initCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func reportErrors(errs []error, format string) {
	if format == "json" {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		b, _ := json.MarshalIndent(map[string]any{"errors": msgs}, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "  ✗", e)
		}
	}
}
