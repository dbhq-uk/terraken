variable "seed" { type = string }
resource "terraform_data" "a" {
  triggers_replace = var.seed
  input            = "a"
}
resource "terraform_data" "hidden" {
  triggers_replace = var.seed
  input            = "hidden"
}
output "a" { value = terraform_data.a.output }
output "b" { value = terraform_data.hidden.output }
