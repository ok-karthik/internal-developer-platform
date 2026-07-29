locals {
    resource_name_prefix = "payments-my-app"
    tags = {
        Team        = "payments"
        Service     = "my-app"
        ManagedBy   = "terraform"
        Owner       = "payments"
    }
}

