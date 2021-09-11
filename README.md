# awslogcheck

It's actually a draft (under development), consider it as a POC for the instant.
The purpose is to create a tool to parse cloudwatch logs and get a mail report with all occurences that doesn't match with regexp given (like logcheck but for AWS EKS application, considering that logs are stored in cloudwatch thanks to fluentd).

Actually, the program can connect to AWS API through SSO profile or get the default config (need to give permissions to the EC2 that will run the program).

awslogcheck will be spawned every hour and if there are logs that do not fit with rules, you will get an email (Need a mailgun account).

# Configuration

The configuration files has the below format :

```
rulesdir: "/opt/awslogcheck/rules-perso"
imagesToIgnore:
  - fluent/fluentd-kubernetes-daemonset
  - 602401143452.dkr.ecr.eu-west-3.amazonaws.com/eks/kube-proxy
  - docker:stable
  - docker:dind
containerNameToIgnore:
  - aws-vpc-cni-init
  - helper
  - build
  - svc-0
```

imagesToIgnore and containerNameToIgnore are golang regexp expression, you can test with [https://regex101.com/](https://regex101.com/)

Some environment variables need to be declared :

```
MAILGUN_APIKEY: "..."
MAILGUN_DOMAIN: "..."
MAILGROM:
MAILTO: 
SUBJECT: "awslogcheck"
AWS_REGION: eu-west-3
LOGGROUP: "/aws/containerinsights/dev-EKS/application"
```

Every environment vars are mandatory. The loggroup should be the loggroup created by fluentd deployment. awslogcheck won't be able to check another structure of event.

![loggroup](img/log-groups.png)

# Deployment in kubernetes

You have manifests example in the deploy folder.

## Development

This tool uses the aws sdk golang v2. [Here is the doc.](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2)

[Most of API calls use cloudwatchlogs.](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs)
