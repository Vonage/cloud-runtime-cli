package initcmd

// longText is the long description, byte for byte as earlier releases print it.
const longText = `Initialize a new VCR project from a template.

This interactive command creates a new VCR project by guiding you through:
  • Project name        - A unique identifier for your project
  • Instance name       - The deployment instance name (e.g., dev, staging, prod)
  • Runtime             - The programming language runtime (e.g., nodejs18, python3)
  • Region              - The Vonage Cloud Runtime region for deployment
  • Application ID      - The Vonage application to link (for deployment)
  • Debug Application   - The Vonage application for debug mode
  • Template            - A starter template for your chosen runtime

OUTPUT
  A vcr.yml manifest file is created with your configuration. This file defines
  how your application is built and deployed to the VCR platform.

TEMPLATES
  Templates provide starter code for common use cases including:
  • Starter projects (basic setup)
  • Voice applications
  • Messaging applications
  • Real-time communication apps

PROJECT NAME REQUIREMENTS
  • Must contain only lowercase letters, numbers, and hyphens
  • Must start and end with an alphanumeric character
`

// exampleText is the example block of the help page.
const exampleText = `# Initialize a project in the current directory
$ vcr init
? Enter your project name: my-project
? Enter your Instance name: dev
? Select a runtime: nodejs18
? Select a region: aws.euw1 - AWS Europe (Ireland)
? Select your Vonage application ID for deployment: my-app (abc123...)
? Select your Vonage application ID for debug: my-debug-app (def456...)
? Select a product template: Starter Project - Node.js
✓ vcr.yml created

# Initialize in a new directory (creates directory if it doesn't exist)
$ vcr init my-new-project

# Initialize using the short alias
$ vcr i my-project
`
