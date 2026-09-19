variable "release" {
  type    = string
  default = "v1"
}

variable "fail" {
  type    = bool
  default = false
}

resource "terraform_data" "base" {
  triggers_replace = var.release
  input            = "base ${var.release}"
}

# Replaced because of the base, create_before_destroy, and its create fails
# once - which is how an object ends up deposed and still in the next plan.
resource "terraform_data" "svc" {
  triggers_replace = terraform_data.base.output
  input            = "svc"

  lifecycle {
    create_before_destroy = true
  }

  provisioner "local-exec" {
    command = var.fail ? "exit 1" : "true"
  }
}
