variable "seed" { type = string }
module "child" {
  source = "../leaf"
  seed   = var.seed
}
output "result" { value = module.child.a }
