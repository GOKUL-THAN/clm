module github.com/m2p/clm/satellite-gateway

go 1.22

require (
	github.com/aws/aws-sdk-go-v2 v1.30.0
	github.com/aws/aws-sdk-go-v2/config v1.27.20
	github.com/aws/aws-sdk-go-v2/credentials v1.17.20
	github.com/aws/aws-sdk-go-v2/service/acm v1.27.0
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.33.0
	github.com/aws/aws-sdk-go-v2/service/route53 v1.40.10
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.31.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.30.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/spf13/viper v1.18.2
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.23.0
)
