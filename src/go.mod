module github.com/sgaunet/awslogcheck

go 1.17

require (
	github.com/aws/aws-sdk-go-v2 v1.8.0
	github.com/aws/aws-sdk-go-v2/config v1.6.0
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.5.2
	github.com/aws/aws-sdk-go-v2/service/sts v1.6.1
	github.com/sirupsen/logrus v1.8.1
	gopkg.in/yaml.v2 v2.2.8
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.3.2 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.2.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.3.2 // indirect
	github.com/aws/smithy-go v1.7.0 // indirect
	golang.org/x/sys v0.0.0-20191026070338-33540a1f6037 // indirect
)
