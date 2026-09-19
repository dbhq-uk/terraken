variable "seed" { type = string }
resource "terraform_data" "backing" {
  triggers_replace = var.seed
  input            = "backing"
}
output "items" { value = [{ id = terraform_data.backing.output }] }
