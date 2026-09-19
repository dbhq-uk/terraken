variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  triggers_replace = var.release
  input            = "base ${var.release}"
}

resource "terraform_data" "middle" {
  triggers_replace = terraform_data.base.output
  input            = "middle"
}

resource "terraform_data" "leaf" {
  triggers_replace = terraform_data.middle.output
  input            = "leaf"
}

resource "terraform_data" "unrelated" {
  input = "unrelated"
}
