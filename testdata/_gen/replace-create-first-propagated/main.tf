variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "upstream" {
  triggers_replace = var.release
  input            = "upstream ${var.release}"
}

resource "terraform_data" "downstream" {
  triggers_replace = terraform_data.upstream.output
  input            = "downstream"

  lifecycle {
    create_before_destroy = true
  }
}
