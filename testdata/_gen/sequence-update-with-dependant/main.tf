variable "release" {
  type    = string
  default = "v1"
}

# Updated in place: its input changes, nothing forces a replacement.
resource "terraform_data" "base" {
  input = "base ${var.release}"
}

# Replaced, because what triggers its replacement is the base's output.
resource "terraform_data" "follower" {
  triggers_replace = terraform_data.base.output
  input            = "follower"
}
