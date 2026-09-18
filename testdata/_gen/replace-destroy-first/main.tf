terraform {
  required_version = ">= 1.9.0"
}

variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "service" {
  triggers_replace = var.release
  input            = "service ${var.release}"
}
