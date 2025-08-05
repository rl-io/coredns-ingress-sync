# Documentation Index

Welcome to the coredns-ingress-sync documentation! This guide helps you find the information you need.

## Quick Navigation

### 🚀 Getting Started

- **[README](../README.md)** - Project overview, quick start, and installation
- **[Configuration Guide](CONFIGURATION.md)** - Detailed configuration options and examples

### 🛠️ Technical Details  

- **[Architecture Guide](ARCHITECTURE.md)** - Technical implementation and design
- **[Development Guide](DEVELOPMENT.md)** - Local development and testing

### 🆘 Support & Help

- **[Troubleshooting Guide](TROUBLESHOOTING.md)** - Common issues and solutions
- **[FAQ](FAQ.md)** - Frequently asked questions

## Documentation by Audience

### For Operations Teams

- **Installation**: [README > Quick Start](../README.md#quick-start)
- **Configuration**: [Configuration Guide](CONFIGURATION.md)
- **Monitoring**: [Troubleshooting Guide > Monitoring](TROUBLESHOOTING.md#monitoring-and-observability)
- **Troubleshooting**: [Troubleshooting Guide](TROUBLESHOOTING.md)

### For Platform Engineers

- **Architecture**: [Architecture Guide](ARCHITECTURE.md)
- **Advanced Configuration**: [Configuration Guide > Advanced](CONFIGURATION.md#advanced-configuration)
- **Integration**: [FAQ > Terraform Integration](FAQ.md#terraform-integration--defensive-configuration)
- **Security**: [Configuration Guide > Security](CONFIGURATION.md#security-configuration)

### For Developers

- **Development Setup**: [Development Guide](DEVELOPMENT.md)
- **Testing**: [Development Guide > Testing](DEVELOPMENT.md#testing)
- **Contributing**: [Development Guide > Contributing](DEVELOPMENT.md#contributing)
- **API Reference**: [Architecture Guide > Controller Implementation](ARCHITECTURE.md#controller-implementation)
- **Debugging**: [Troubleshooting Guide > Advanced Debugging](TROUBLESHOOTING.md#advanced-debugging)

### For DevOps Teams

- **CI/CD Integration**: [Development Guide > CI/CD](DEVELOPMENT.md#ci-cd-integration)
- **Helm Chart**: [Configuration Guide > Helm Values](CONFIGURATION.md#helm-values-reference)
- **Deployment Strategies**: [Configuration Guide > Production](CONFIGURATION.md#production-configuration)

## Documentation Topics

### Installation & Setup

| Topic | Guide | Section |
|-------|-------|---------|
| Basic Installation | [README](../README.md) | Quick Start |
| Helm Configuration | [Configuration Guide](CONFIGURATION.md) | Helm Values Reference |
| Environment Setup | [Development Guide](DEVELOPMENT.md) | Development Environment |

### Configuration & Customization

| Topic | Guide | Section |
|-------|-------|---------|
| Basic Configuration | [Configuration Guide](CONFIGURATION.md) | Basic Configuration |
| Advanced Options | [Configuration Guide](CONFIGURATION.md) | Advanced Configuration |
| Security Settings | [Configuration Guide](CONFIGURATION.md) | Security Configuration |
| Multiple Environments | [Configuration Guide](CONFIGURATION.md) | Environment-Specific |

### Technical Implementation

| Topic | Guide | Section |
|-------|-------|---------|
| How It Works | [Architecture Guide](ARCHITECTURE.md) | Overview |
| Controller Logic | [Architecture Guide](ARCHITECTURE.md) | Controller Implementation |
| CoreDNS Integration | [Architecture Guide](ARCHITECTURE.md) | CoreDNS Integration |
| Performance | [Architecture Guide](ARCHITECTURE.md) | Performance Characteristics |

### Operations & Maintenance

| Topic | Guide | Section |
|-------|-------|---------|
| Monitoring | [Troubleshooting Guide](TROUBLESHOOTING.md) | Monitoring and Observability |
| Common Issues | [Troubleshooting Guide](TROUBLESHOOTING.md) | Common Issues and Solutions |
| Debugging | [Troubleshooting Guide](TROUBLESHOOTING.md) | Advanced Debugging |
| FAQ | [FAQ](FAQ.md) | All Topics |

### Development & Testing

| Topic | Guide | Section |
|-------|-------|---------|
| Local Development | [Development Guide](DEVELOPMENT.md) | Development Environment |
| Testing Framework | [Development Guide](DEVELOPMENT.md) | Testing |
| Code Standards | [Development Guide](DEVELOPMENT.md) | Code Standards |
| Contributing | [Development Guide](DEVELOPMENT.md) | Contributing |
| Debugging | [Troubleshooting Guide](TROUBLESHOOTING.md) | Advanced Debugging |

## Common Use Cases

### First-Time Installation

1. [README > Prerequisites](../README.md#prerequisites)
2. [README > Installation](../README.md#installation)
3. [README > Verification](../README.md#verification)
4. [Configuration Guide > Basic Configuration](CONFIGURATION.md#basic-configuration)

### Production Deployment

1. [Configuration Guide > Production Configuration](CONFIGURATION.md#production-configuration)
2. [Configuration Guide > Security Configuration](CONFIGURATION.md#security-configuration)
3. [Architecture Guide > High Availability](ARCHITECTURE.md#high-availability)
4. [Troubleshooting Guide > Monitoring](TROUBLESHOOTING.md#monitoring-and-observability)

### Troubleshooting Issues

1. [Troubleshooting Guide > Quick Diagnostic Steps](TROUBLESHOOTING.md#quick-diagnostic-steps)
2. [Troubleshooting Guide > Common Issues](TROUBLESHOOTING.md#common-issues-and-solutions)
3. [FAQ](FAQ.md)
4. [Troubleshooting Guide > Getting Help](TROUBLESHOOTING.md#getting-help)

### Development Contribution

1. [Development Guide > Development Environment](DEVELOPMENT.md#development-environment)
2. [Development Guide > Testing](DEVELOPMENT.md#testing)
3. [Development Guide > Contributing](DEVELOPMENT.md#contributing)
4. [Architecture Guide > Controller Implementation](ARCHITECTURE.md#controller-implementation)

## External Resources

### Kubernetes Documentation

- [Ingress Controllers](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/)
- [CoreDNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/)
- [ConfigMaps](https://kubernetes.io/docs/concepts/configuration/configmap/)

### CoreDNS Documentation  

- [CoreDNS Manual](https://coredns.io/manual/toc/)
- [Rewrite Plugin](https://coredns.io/plugins/rewrite/)
- [Import Plugin](https://coredns.io/plugins/import/)

### Controller Runtime

- [Controller Runtime Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubebuilder Book](https://book.kubebuilder.io/)

## Getting Help

### Quick Questions

- Check the [FAQ](FAQ.md) first
- Search [existing issues](https://github.com/rl-io/coredns-ingress-sync/issues)

### Technical Support

- [GitHub Issues](https://github.com/rl-io/coredns-ingress-sync/issues) for bugs and feature requests
- [GitHub Repository](https://github.com/rl-io/coredns-ingress-sync) for general inquiries

### Contributing

- Read the [Development Guide](DEVELOPMENT.md)
- Review [contribution guidelines](DEVELOPMENT.md#contributing)
- Submit [pull requests](https://github.com/rl-io/coredns-ingress-sync/pulls)

## Document Maintenance

This documentation is maintained alongside the codebase. When making changes:

1. Update relevant documentation files
2. Test documentation examples
3. Update this index if adding new sections
4. Ensure cross-references remain valid

### Last Updated

- Documentation restructured: 2025-01-16
- All guides: Current with v0.1.14
- Examples tested: 2025-01-16
