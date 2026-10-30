package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	apiURL   string
	apiToken string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "clm",
		Short: "CLM - Certificate Lifecycle Management CLI",
		Long:  "Manage certificates across multi-cloud, multi-OS environments with satellite-based deployment.",
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", getEnvOrDefault("CLM_API_URL", "http://localhost:8080"), "CLM SaaS API URL")
	rootCmd.PersistentFlags().StringVar(&apiToken, "token", os.Getenv("CLM_TOKEN"), "API authentication token")

	// Register sub-commands
	rootCmd.AddCommand(
		newLoginCmd(),
		newCertCmd(),
		newSatelliteCmd(),
		newDiscoverCmd(),
		newDeployCmd(),
		newPurchaseCmd(),
		newApprovalCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newLoginCmd() *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with CLM SaaS",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiPost("/api/v1/auth/login", map[string]string{
				"email":    email,
				"password": password,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Login successful. Token: %s\n", resp["access_token"])
			fmt.Println("Export it: export CLM_TOKEN=<token>")
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "User email")
	cmd.Flags().StringVar(&password, "password", "", "User password")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}

func newCertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "Manage certificates",
	}

	// cert create
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Request a new certificate",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			certType, _ := cmd.Flags().GetString("type")
			env, _ := cmd.Flags().GetString("env")
			satellite, _ := cmd.Flags().GetString("satellite")
			ca, _ := cmd.Flags().GetString("ca")

			resp, err := apiPost("/api/v1/certificates", map[string]interface{}{
				"domain":       domain,
				"type":         certType,
				"environment":  env,
				"satellite_id": satellite,
				"ca_provider":  ca,
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	createCmd.Flags().String("domain", "", "Domain name (e.g., *.example.com)")
	createCmd.Flags().String("type", "DV", "Certificate type (DV, OV, EV, wildcard)")
	createCmd.Flags().String("env", "prod", "Environment (prod, staging, dev)")
	createCmd.Flags().String("satellite", "", "Target satellite ID")
	createCmd.Flags().String("ca", "letsencrypt", "CA provider")
	_ = createCmd.MarkFlagRequired("domain")
	_ = createCmd.MarkFlagRequired("satellite")

	// cert list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List certificates",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiGet("/api/v1/certificates")
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}

	// cert revoke
	revokeCmd := &cobra.Command{
		Use:   "revoke [cert-id]",
		Short: "Revoke a certificate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiPost("/api/v1/certificates/"+args[0]+"/revoke", nil)
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}

	cmd.AddCommand(createCmd, listCmd, revokeCmd)
	return cmd
}

func newSatelliteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "satellite",
		Short: "Manage satellites",
	}

	registerCmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new satellite",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			mode, _ := cmd.Flags().GetString("mode")
			cloud, _ := cmd.Flags().GetString("cloud")
			secrets, _ := cmd.Flags().GetString("secrets-backend")

			resp, err := apiPost("/api/v1/satellites", map[string]string{
				"name":            name,
				"type":            "local",
				"connector_mode":  mode,
				"cloud":           cloud,
				"secrets_backend": secrets,
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	registerCmd.Flags().String("name", "", "Satellite name")
	registerCmd.Flags().String("mode", "direct", "Connector mode (direct, proxy, vpn, offline)")
	registerCmd.Flags().String("cloud", "aws", "Cloud provider")
	registerCmd.Flags().String("secrets-backend", "aws", "Secrets backend (aws, gcp, azure, vault)")
	_ = registerCmd.MarkFlagRequired("name")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List satellites",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiGet("/api/v1/satellites")
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status [satellite-id]",
		Short: "Get satellite status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiGet("/api/v1/satellites/" + args[0])
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}

	cmd.AddCommand(registerCmd, listCmd, statusCmd)
	return cmd
}

func newDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover certificates in network/infrastructure",
		RunE: func(cmd *cobra.Command, args []string) error {
			satellite, _ := cmd.Flags().GetString("satellite")
			network, _ := cmd.Flags().GetString("scan-network")

			fmt.Printf("Triggering discovery on satellite %s for network %s...\n", satellite, network)
			// In production: queue a discover command to the satellite
			resp, err := apiPost("/api/v1/satellites/"+satellite+"/commands", map[string]interface{}{
				"type":    "discover",
				"payload": fmt.Sprintf(`{"network":"%s"}`, network),
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	cmd.Flags().String("satellite", "", "Satellite to run discovery on")
	cmd.Flags().String("scan-network", "", "CIDR range to scan")
	_ = cmd.MarkFlagRequired("satellite")
	return cmd
}

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a certificate to targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			certID, _ := cmd.Flags().GetString("cert-id")
			satellite, _ := cmd.Flags().GetString("satellite")
			target, _ := cmd.Flags().GetString("target")
			host, _ := cmd.Flags().GetString("host")

			fmt.Printf("Deploying cert %s via satellite %s to %s (%s)...\n", certID, satellite, target, host)
			resp, err := apiPost("/api/v1/satellites/"+satellite+"/commands", map[string]interface{}{
				"type": "deploy_cert",
				"payload": fmt.Sprintf(`{"cert_id":"%s","target":{"type":"%s","host":"%s"}}`,
					certID, target, host),
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	cmd.Flags().String("cert-id", "", "Certificate ID to deploy")
	cmd.Flags().String("satellite", "", "Satellite to deploy through")
	cmd.Flags().String("target", "", "Target type (linux-nginx, postgresql, aws-alb)")
	cmd.Flags().String("host", "", "Target host")
	_ = cmd.MarkFlagRequired("cert-id")
	_ = cmd.MarkFlagRequired("satellite")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func newPurchaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purchase",
		Short: "Purchase certificates from CAs",
	}

	quoteCmd := &cobra.Command{
		Use:   "quote",
		Short: "Get price quotes from CAs",
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, _ := cmd.Flags().GetString("domain")
			certType, _ := cmd.Flags().GetString("type")
			validity, _ := cmd.Flags().GetString("validity")

			resp, err := apiPost("/api/v1/purchases/quote", map[string]string{
				"domain":    domain,
				"cert_type": certType,
				"validity":  validity,
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	quoteCmd.Flags().String("domain", "", "Domain")
	quoteCmd.Flags().String("type", "DV", "Certificate type")
	quoteCmd.Flags().String("validity", "1y", "Validity period")
	_ = quoteCmd.MarkFlagRequired("domain")

	cmd.AddCommand(quoteCmd)
	return cmd
}

func newApprovalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approval",
		Short: "Manage approval requests",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List pending approvals",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := apiGet("/api/v1/approvals/pending")
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}

	approveCmd := &cobra.Command{
		Use:   "approve [request-id]",
		Short: "Approve a request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			comment, _ := cmd.Flags().GetString("comment")
			resp, err := apiPost("/api/v1/approvals/"+args[0]+"/approve", map[string]string{
				"comment": comment,
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	approveCmd.Flags().String("comment", "", "Approval comment")

	rejectCmd := &cobra.Command{
		Use:   "reject [request-id]",
		Short: "Reject a request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reason, _ := cmd.Flags().GetString("reason")
			resp, err := apiPost("/api/v1/approvals/"+args[0]+"/reject", map[string]string{
				"comment": reason,
			})
			if err != nil {
				return err
			}
			printJSON(resp)
			return nil
		},
	}
	rejectCmd.Flags().String("reason", "", "Rejection reason")
	_ = rejectCmd.MarkFlagRequired("reason")

	cmd.AddCommand(listCmd, approveCmd, rejectCmd)
	return cmd
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
