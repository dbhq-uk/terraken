variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  triggers_replace = var.release
  input            = "base ${var.release}"
}

resource "terraform_data" "follower" {
  input = terraform_data.base.output
}
