variable "seed" {
  type = string
}

resource "terraform_data" "inner" {
  triggers_replace = var.seed
  input            = "inner"
}

output "handle" {
  value = terraform_data.inner.output
}
