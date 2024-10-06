target "release" {
  tags = ["docker.io/ericmiller/healthchecksio-cli"]
  platforms = ["linux/amd64", "linux/arm64"]
  pull = true
}
