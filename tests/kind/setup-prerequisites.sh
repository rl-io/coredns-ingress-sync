#!/bin/bash
# Quick setup script for KIND multi-version testing prerequisites

set -euo pipefail

log() {
    echo -e "\033[0;34m[SETUP]\033[0m $*"
}

log_success() {
    echo -e "\033[0;32m✅\033[0m $*"
}

log_error() {
    echo -e "\033[0;31m❌\033[0m $*"
}

log "Setting up KIND multi-version testing prerequisites..."
echo ""

# Check OS
if [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macOS"
    PACKAGE_MANAGER="brew"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="Linux"
    if command -v apt-get &> /dev/null; then
        PACKAGE_MANAGER="apt"
    elif command -v yum &> /dev/null; then
        PACKAGE_MANAGER="yum"
    else
        log_error "Unsupported Linux distribution. Please install manually."
        exit 1
    fi
else
    log_error "Unsupported OS: $OSTYPE"
    exit 1
fi

log "Detected OS: $OS with package manager: $PACKAGE_MANAGER"

# Check and install Docker
if ! command -v docker &> /dev/null; then
    log "Docker not found. Installing..."
    case $PACKAGE_MANAGER in
        brew)
            brew install --cask docker
            ;;
        apt)
            curl -fsSL https://get.docker.com -o get-docker.sh
            sudo sh get-docker.sh
            sudo usermod -aG docker $USER
            rm get-docker.sh
            ;;
        yum)
            sudo yum install -y docker
            sudo systemctl start docker
            sudo systemctl enable docker
            sudo usermod -aG docker $USER
            ;;
    esac
    log_success "Docker installed"
else
    log_success "Docker already installed"
fi

# Check and install KIND
if ! command -v kind &> /dev/null; then
    log "KIND not found. Installing..."
    case $PACKAGE_MANAGER in
        brew)
            brew install kind
            ;;
        apt|yum)
            # Install for Linux using binary download
            curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.23.0/kind-linux-amd64
            chmod +x ./kind
            sudo mv ./kind /usr/local/bin/kind
            ;;
    esac
    log_success "KIND installed"
else
    log_success "KIND already installed"
fi

# Check and install kubectl
if ! command -v kubectl &> /dev/null; then
    log "kubectl not found. Installing..."
    case $PACKAGE_MANAGER in
        brew)
            brew install kubectl
            ;;
        apt)
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
            chmod +x kubectl
            sudo mv kubectl /usr/local/bin/
            ;;
        yum)
            cat <<EOF | sudo tee /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v1.30/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v1.30/rpm/repodata/repomd.xml.key
EOF
            sudo yum install -y kubectl
            ;;
    esac
    log_success "kubectl installed"
else
    log_success "kubectl already installed"
fi

# Check and install Helm
if ! command -v helm &> /dev/null; then
    log "Helm not found. Installing..."
    case $PACKAGE_MANAGER in
        brew)
            brew install helm
            ;;
        apt|yum)
            curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
            ;;
    esac
    log_success "Helm installed"
else
    log_success "Helm already installed"
fi

echo ""
log_success "All prerequisites installed successfully!"
echo ""

# Check Docker daemon
if ! docker info &> /dev/null; then
    log_error "Docker daemon is not running. Please start Docker."
    if [[ "$OS" == "macOS" ]]; then
        echo "Start Docker Desktop application"
    else
        echo "Run: sudo systemctl start docker"
    fi
    echo ""
fi

# Show usage
log "You can now run KIND multi-version tests:"
echo ""
echo "  make kind-test-list                    # List supported versions"
echo "  make kind-test-latest                  # Test latest K8s version"
echo "  make kind-test-version K8S_VERSION=1.29.4   # Test specific version"
echo "  make kind-test-all-versions            # Test all supported versions"
echo "  make kind-test-cleanup                 # Clean up test clusters"
echo ""
echo "See tests/kind/README.md for detailed documentation."

# Group membership note for Linux
if [[ "$OS" == "Linux" ]] && [[ "$PACKAGE_MANAGER" != "brew" ]]; then
    echo ""
    log "Note: You may need to log out and back in for Docker group membership to take effect."
fi
