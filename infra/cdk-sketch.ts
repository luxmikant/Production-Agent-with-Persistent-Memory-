# AWS CDK sketch (production path)

The docker-compose path is recommended for the demo. For production, the
following CDK stack replaces it. Snapshots only — adjust to your account.

```ts
// infra/cdk/bin/stillmind.ts  (sketch)
import * as cdk from "aws-cdk-lib";
import { Stack } from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as apigw from "aws-cdk-lib/aws-apigateway";
import * as s3 from "aws-cdk-lib/aws-s3";
import * as eventbridge from "aws-cdk-lib/aws-events";
import * as targets from "aws-cdk-lib/aws-events-targets";

class StillMindStack extends Stack {
  constructor(scope: cdk.App, id: string) {
    super(scope, id);

    const artifacts = new s3.Bucket(this, "Artifacts");

    const api = new lambda.Function(this, "API", {
      runtime: lambda.Runtime.GO_1_X,
      handler: "stillmind",
      code: lambda.Code.fromAsset("../backend/bin"),
      memorySize: 512,
      timeout: cdk.Duration.seconds(30),
      environment: {
        CRDB_URL: process.env.CRDB_URL!,
        AWS_REGION: process.env.AWS_REGION!,
        BEDROCK_MODEL: "anthropic.claude-3-haiku-20240307-v1:0",
        BEDROCK_EMBED_MODEL: "amazon.titan-embed-text-v2:0",
      },
    });

    const compact = new lambda.Function(this, "Compact", {
      runtime: lambda.Runtime.GO_1_X,
      handler: "stillmind-compact",
      code: lambda.Code.fromAsset("../backend/bin"),
      memorySize: 512,
      timeout: cdk.Duration.minutes(5),
    });

    new eventbridge.Rule(this, "CompactEvery5", {
      schedule: eventbridge.Schedule.rate(cdk.Duration.minutes(5)),
      targets: [new targets.LambdaFunction(compact)],
    });

    new apigw.LambdaRestApi(this, "Gateway", { handler: api });
  }
}

new StillMindStack(new cdk.App(), "StillMind");
```

**Notes**
- Frontend is hosted on S3 + CloudFront (or Amplify) — separate stack.
- CockroachDB is provisioned via CockroachDB Cloud (free tier is eligible for the hackathon).
- IAM: both Lambdas have an inline policy allowing `bedrock:InvokeModel` on the pinned model ARNs.
