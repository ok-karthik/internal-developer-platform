# Variables
CLUSTER_PROVIDER ?= k3d
CLUSTER_NAME ?= nexus-platform
AWS_CREDS ?= ./aws-creds.ini

.PHONY: help check-deps create-cluster delete-cluster install-argocd bootstrap configure-aws up setup clean destroy get-argocd-creds wait-for-apps install-scaffolder run-api demo-onboard-team demo-add-service fire-synthetic-alert print-kubeconfig-stanza

# Default target: show help
help:
	@echo "GitOps IDP Blueprint Makefile"
	@echo "Usage: make <target> [CLUSTER_PROVIDER=<provider>] [CLUSTER_NAME=<name>]"
	@echo ""
	@echo "Supported Providers (CLUSTER_PROVIDER):"
	@echo "  k3d       - (Default) Provisions a local K3d cluster with Traefik disabled"
	@echo "  orbstack  - Starts/Manages OrbStack's built-in Kubernetes engine"
	@echo "  minikube  - Starts Minikube cluster and enables ingress addon"
	@echo "  kind      - Provisions a Kind cluster with port mapping configuration"
	@echo "  existing  - Targets your current kubectl context without provisioning a new cluster"
	@echo ""
	@echo "Targets:"
	@echo "  up / setup      - Full setup: check deps, create cluster, install ArgoCD, bootstrap platform, configure AWS"
	@echo "  create-cluster  - Provision/start Kubernetes cluster using specified provider"
	@echo "  delete-cluster  - Delete/reset the provisioned Kubernetes cluster"
	@echo "  install-argocd  - Install ArgoCD via Helm"
	@echo "  bootstrap       - Bootstrap platform applications using ArgoCD"
	@echo "  configure-aws   - Create AWS credentials secret in crossplane-system namespace"
	@echo "  clean           - Remove deployed components (keep cluster)"
	@echo "  destroy         - Full teardown of components and cluster"
	@echo "  check-deps      - Validate required command-line utilities"
	@echo "  get-argocd-creds - Display ArgoCD login URL, username, and password"
	@echo "  fire-synthetic-alert - POST a synthetic alert through Alertmanager's routing tree"
	@echo ""
	@echo "Examples:"
	@echo "  make setup CLUSTER_PROVIDER=orbstack"
	@echo "  make destroy CLUSTER_PROVIDER=orbstack"
	@echo "  make bootstrap"

# Conditional commands based on CLUSTER_PROVIDER
#
# Phase 7.2 note: the local cluster does NOT pass --oidc-issuer-url etc. via
# --k3s-arg, even though that is the documented mechanism for wiring the API
# server to Keycloak on k3d. Reason: k3d sets API-server flags at cluster
# CREATION time, but Keycloak is installed AFTER the cluster exists (it is an
# ArgoCD-managed addon in 2-cluster-services/identity/, applied by `make
# bootstrap`, which itself needs the cluster first). Enabling OIDC flags at
# `create-cluster` would point the API server at an issuer URL that does not
# resolve yet. A real two-phase bootstrap (create cluster -> install Keycloak
# -> recreate the cluster with OIDC flags pointed at it) would work but is
# more machinery than this local harness is worth — see the Target
# Environment section in PLAN.md: the honest fix is EKS Access Entries
# (7.2b), which have no such chicken-and-egg problem because they are not
# API-server startup flags at all. The flags below are commented, for the
# concept demo:
#   --k3s-arg "--kube-apiserver-arg=oidc-issuer-url=https://keycloak.localhost/realms/platform@server:*"
#   --k3s-arg "--kube-apiserver-arg=oidc-client-id=kubernetes@server:*"
#   --k3s-arg "--kube-apiserver-arg=oidc-username-claim=preferred_username@server:*"
#   --k3s-arg "--kube-apiserver-arg=oidc-username-prefix=oidc:@server:*"
#   --k3s-arg "--kube-apiserver-arg=oidc-groups-claim=groups@server:*"
#   --k3s-arg "--kube-apiserver-arg=oidc-groups-prefix=oidc:@server:*"
ifeq ($(CLUSTER_PROVIDER),k3d)
CREATE_CLUSTER_CMD = k3d cluster create $(CLUSTER_NAME) \
	--k3s-arg "--disable=traefik@server:*" \
	-p "80:80@loadbalancer" -p "443:443@loadbalancer"
DELETE_CLUSTER_CMD = k3d cluster delete $(CLUSTER_NAME)
else ifeq ($(CLUSTER_PROVIDER),orbstack)
CREATE_CLUSTER_CMD = orbctl start k8s
DELETE_CLUSTER_CMD = orbctl delete k8s
else ifeq ($(CLUSTER_PROVIDER),minikube)
CREATE_CLUSTER_CMD = minikube start --profile $(CLUSTER_NAME) && minikube addons enable ingress --profile $(CLUSTER_NAME)
DELETE_CLUSTER_CMD = minikube delete --profile $(CLUSTER_NAME)
else ifeq ($(CLUSTER_PROVIDER),kind)
CREATE_CLUSTER_CMD = printf 'apiVersion: kind.x-k8s.io/v1alpha4\nkind: Cluster\nnodes:\n- role: control-plane\n  extraPortMappings:\n  - containerPort: 80\n    hostPort: 80\n    listenAddress: "127.0.0.1"\n    protocol: TCP\n  - containerPort: 443\n    hostPort: 443\n    listenAddress: "127.0.0.1"\n    protocol: TCP\n' | kind create cluster --name $(CLUSTER_NAME) --config=-
DELETE_CLUSTER_CMD = kind delete cluster --name $(CLUSTER_NAME)
else
CREATE_CLUSTER_CMD = @echo "Using existing Kubernetes cluster (context: \$$(kubectl config current-context))"
DELETE_CLUSTER_CMD = @echo "Skipping cluster deletion for existing/external cluster"
endif

check-deps:
	@echo "Checking dependencies..."
	@which kubectl > /dev/null || (echo "Error: kubectl is not installed" && exit 1)
	@which helm > /dev/null || (echo "Error: helm is not installed" && exit 1)
ifeq ($(CLUSTER_PROVIDER),k3d)
	@which k3d > /dev/null || (echo "Error: k3d is not installed" && exit 1)
else ifeq ($(CLUSTER_PROVIDER),orbstack)
	@which orbctl > /dev/null || (echo "Error: orbctl is not installed" && exit 1)
else ifeq ($(CLUSTER_PROVIDER),minikube)
	@which minikube > /dev/null || (echo "Error: minikube is not installed" && exit 1)
else ifeq ($(CLUSTER_PROVIDER),kind)
	@which kind > /dev/null || (echo "Error: kind is not installed" && exit 1)
endif
	@echo "All dependencies satisfied!"

create-cluster: check-deps
	@echo "Creating/starting cluster using $(CLUSTER_PROVIDER) provider..."
	$(CREATE_CLUSTER_CMD)
	@echo "Waiting for Kubernetes API server to be reachable..."
	@for i in {1..30}; do \
		kubectl get nodes >/dev/null 2>&1 && break; \
		printf "."; \
		sleep 2; \
	done; echo ""
	@if ! kubectl get nodes >/dev/null 2>&1; then \
		echo "Error: Kubernetes API server is unreachable."; \
		exit 1; \
	fi
	@echo "Kubernetes API server is ready!"
ifeq ($(CLUSTER_PROVIDER),minikube)
	@echo "============================================================"
	@echo "NOTE: When using minikube on macOS, you may need to run:"
	@echo "      sudo minikube tunnel --profile $(CLUSTER_NAME)"
	@echo "      in a separate terminal to expose services on localhost."
	@echo "============================================================"
endif

delete-cluster:
	@echo "Deleting cluster..."
	$(DELETE_CLUSTER_CMD)

install-argocd:
	@echo "Adding ArgoCD Helm repository..."
	helm repo add argo https://argoproj.github.io/argo-helm
	helm repo update
	@echo "Installing/Upgrading ArgoCD..."
	# metrics.enabled + the two serviceMonitor flags expose argocd_app_sync_total
	# and friends to the kube-prometheus-stack Prometheus (its default
	# serviceMonitorSelector has no label restriction, so any ServiceMonitor in
	# the cluster is picked up) — this is what makes the Phase 6 DORA dashboard's
	# deployment-frequency and change-failure-rate panels queryable at all.
	#
	# configs.cm.oidc\.config + configs.rbac.policy.csv wire ArgoCD's OWN login
	# (Phase 7.3) to Keycloak's `argocd` client — this is a THIRD, independent
	# OIDC usage from the Kubernetes-API-server one in create-cluster's comment
	# above and the workload-identity one in 1-cloud-foundation/aws/workload-identity/;
	# see the Four Planes table in .agents/AGENTS.md for why these do not
	# collapse into one config. Written to a temp file rather than piped via a
	# Makefile heredoc: GNU Make strips a leading tab from every recipe line,
	# including heredoc body lines, which silently flattens YAML indentation.
	@printf '%s\n' \
		'configs:' \
		'  cm:' \
		'    url: "https://argocd.localhost"' \
		'    oidc.config: |' \
		'      name: Keycloak' \
		'      issuer: https://keycloak.localhost/realms/platform' \
		'      clientID: argocd' \
		'      clientSecret: $$oidc.keycloak.clientSecret' \
		'      requestedScopes: ["openid", "profile", "email", "groups"]' \
		'  rbac:' \
		'    policy.default: role:readonly' \
		'    policy.csv: |' \
		'      # Regenerated per team by onboard-team once the scaffolder consumes' \
		'      # this (documented follow-up, Phase 7.3 -- 2-idp-scaffolder is out of' \
		'      # scope on this branch). team-a rows below are the seed pattern: copy-' \
		'      # paste per team until that automation exists.' \
		'      p, role:team-a-developer, applications, get,      team-a/*, allow' \
		'      p, role:team-a-developer, applications, sync,     team-a/*, allow' \
		'      p, role:team-a-developer, logs,         get,      team-a/*, allow' \
		'      p, role:team-a-developer, applications, delete,   team-a/*, deny' \
		'      p, role:team-a-developer, applications, override, team-a/*, deny' \
		'      g, platform:team-a:developer, role:team-a-developer' \
		> /tmp/idp-argocd-values.yaml
	helm upgrade --install argocd argo/argo-cd \
		--namespace argocd \
		--reuse-values \
		--set server.extraArgs="{--insecure}" \
		--set metrics.enabled=true \
		--set metrics.serviceMonitor.enabled=true \
		--set controller.metrics.enabled=true \
		--set controller.metrics.serviceMonitor.enabled=true \
		--create-namespace \
		-f /tmp/idp-argocd-values.yaml

bootstrap:
	@echo "Bootstrapping platform..."
	kubectl apply -f bootstrap.yaml
	@echo "Triggering immediate refresh/sync on all ArgoCD applications..."
	@sleep 2
	@kubectl get app -n argocd -o name 2>/dev/null | xargs -I {} kubectl patch {} -n argocd --type merge -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"normal"}}}' 2>/dev/null || true

configure-aws:
	@echo "Ensuring crossplane-system namespace exists..."
	kubectl create namespace crossplane-system --dry-run=client -o yaml | kubectl apply -f -
	@echo "Creating/updating AWS credentials secret..."
	kubectl create secret generic aws-creds -n crossplane-system --from-file=creds=$(AWS_CREDS) --dry-run=client -o yaml | kubectl apply -f -

up setup: create-cluster install-argocd bootstrap wait-for-apps configure-aws get-argocd-creds
	@echo "Platform setup completed successfully!"

wait-for-apps:
	@echo "Waiting for ArgoCD root application to sync..."
	@for i in {1..30}; do \
		STATUS=$$(kubectl -n argocd get app platform-bootstrap -o jsonpath="{.status.sync.status}" 2>/dev/null || echo "Unknown"); \
		if [ "$$STATUS" = "Synced" ]; then break; fi; \
		printf "."; \
		sleep 3; \
	done; echo ""
	@echo "Waiting for all nested ArgoCD sub-applications to sync and become healthy..."
	@echo "This may take a couple of minutes as images are pulled and CRDs are created."
	@SUCCESS=false; \
	for i in {1..60}; do \
		SYNC_STATUSES=$$(kubectl -n argocd get app -o jsonpath='{range .items[*]}{.status.sync.status}{"\n"}{end}' 2>/dev/null); \
		HEALTH_STATUSES=$$(kubectl -n argocd get app -o jsonpath='{range .items[*]}{.status.health.status}{"\n"}{end}' 2>/dev/null); \
		TOTAL=$$(echo "$$SYNC_STATUSES" | grep -v "^$$" | wc -l | tr -d ' ' || echo 0); \
		SYNCED=$$(echo "$$SYNC_STATUSES" | grep -c "Synced" || echo 0); \
		HEALTHY=$$(echo "$$HEALTH_STATUSES" | grep -c "Healthy" || echo 0); \
		echo "--------------------------------------------------------------------------------"; \
		echo "Progress: $$SYNCED/$$TOTAL apps synced, $$HEALTHY/$$TOTAL apps healthy"; \
		echo "--------------------------------------------------------------------------------"; \
		kubectl -n argocd get app -o custom-columns=NAME:.metadata.name,SYNC:.status.sync.status,HEALTH:.status.health.status 2>/dev/null || true; \
		if [ "$$TOTAL" -gt 1 ] && [ "$$SYNCED" -eq "$$TOTAL" ] && [ "$$HEALTHY" -eq "$$TOTAL" ]; then \
			echo "All $$TOTAL ArgoCD applications are Synced and Healthy!"; \
			SUCCESS=true; \
			break; \
		fi; \
		sleep 8; \
	done; \
	if [ "$$SUCCESS" != "true" ]; then \
		echo "Error: ArgoCD applications failed to sync/become healthy in time."; \
		exit 1; \
	fi

get-argocd-creds:
	@echo "Waiting for ArgoCD initial admin secret to be generated..."
	@for i in {1..30}; do \
		kubectl -n argocd get secret argocd-initial-admin-secret >/dev/null 2>&1 && break; \
		printf "."; \
		sleep 2; \
	done; echo ""
	@if ! kubectl -n argocd get secret argocd-initial-admin-secret >/dev/null 2>&1; then \
		echo "Error: ArgoCD initial admin secret was not generated in time."; \
		exit 1; \
	fi
	@echo "===================================================="
	@echo "ArgoCD Access Information:"
	@echo "URL: http://argocd.localhost"
	@echo "Username: admin"
	@printf "Password: "
	@kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
	@echo ""
	@echo "===================================================="

clean:
	@echo "Cleaning up deployed components..."
	kubectl delete -f bootstrap.yaml --ignore-not-found=true
	helm uninstall argocd -n argocd || true
	kubectl delete namespace argocd --ignore-not-found=true
	kubectl delete secret aws-creds -n crossplane-system --ignore-not-found=true

destroy:
ifeq ($(CLUSTER_PROVIDER),existing)
	$(MAKE) clean
else
	$(MAKE) delete-cluster
endif
	@echo "Platform completely destroyed!"

# --- Synthetic alert (Phase 4.3) ---------------------------------------------
#
# Proves the Alertmanager routing tree in
# 4-platform-engineering/2-cluster-services/observability/prometheus.yaml (the
# alertmanager.config route/receivers/inhibit_rules) actually delivers, without
# waiting for a real burn-rate breach. Posts directly to Alertmanager's API —
# `alertmanager-operated` is the headless Service the Prometheus Operator
# always creates for an Alertmanager CR, named the same regardless of Helm
# release name, so this does not depend on the chart's generated Service name.
.PHONY: fire-synthetic-alert
fire-synthetic-alert:
	@echo "Port-forwarding Alertmanager (ctrl-c has no effect until curl returns)..."
	@kubectl -n monitoring port-forward svc/alertmanager-operated 9093:9093 >/tmp/idp-am-portforward.log 2>&1 & \
	PF_PID=$$!; \
	sleep 3; \
	echo "Firing a synthetic 'page' alert for app-a..."; \
	curl -s -XPOST http://localhost:9093/api/v2/alerts -H 'Content-Type: application/json' -d '[{ \
	  "labels": {"alertname":"SyntheticTestAlert","severity":"page","namespace":"team-a","sloi":"app-a-availability"}, \
	  "annotations": {"summary":"Synthetic alert fired by make fire-synthetic-alert"}, \
	  "startsAt": "'$$(date -u +%Y-%m-%dT%H:%M:%S.000Z)'" \
	}]'; \
	echo ""; \
	kill $$PF_PID 2>/dev/null || true
	@echo "Check the receiver: kubectl -n monitoring logs deploy/alert-webhook-receiver | tail -20"

# --- Onboarding artefact (Phase 7.2) -----------------------------------------
#
# The kubeconfig stanza a new developer pastes to authenticate via OIDC
# (kubectl oidc-login / kubelogin as a client-go credential plugin) rather
# than a static token. Same stanza shape regardless of which of the three
# control surfaces (7.2's raw flags, 7.2b's EKS Access Entries, or an
# AKS/GKE managed OIDC issuer) is behind it — that portability is the point.
.PHONY: print-kubeconfig-stanza
print-kubeconfig-stanza:
	@echo "Add this user entry to ~/.kube/config (or run: kubectl config set-credentials platform-oidc --exec-command=kubectl ...):"
	@echo ""
	@echo "users:"
	@echo "- name: platform-oidc"
	@echo "  user:"
	@echo "    exec:"
	@echo "      apiVersion: client.authentication.k8s.io/v1beta1"
	@echo "      command: kubectl"
	@echo "      args:"
	@echo "        - oidc-login"
	@echo "        - get-token"
	@echo "        - --oidc-issuer-url=https://keycloak.localhost/realms/platform"
	@echo "        - --oidc-client-id=kubernetes"
	@echo "        - --oidc-extra-scope=groups"
	@echo ""
	@echo "Requires the kubelogin plugin: kubectl krew install oidc-login"

install-scaffolder:
	cd 2-idp-scaffolder/python && uv pip install -e .

run-api:
	cd 2-idp-scaffolder/python && fastapi dev api.py

# --- Go scaffolder demo ------------------------------------------------------
#
# The CLI defaults --output-root to the current directory and appends nothing to
# it — correct for a client standing in their own repo. This repo's demo flow
# wants the output under 3-tenant-workloads/, so that path is named HERE rather
# than compiled into the binary. `git rev-parse --show-toplevel` is git's own
# root-finder and works from any subdirectory.
#
# --catalog-root is passed too, so local edits to 1-platform-catalog/ take effect
# without pushing; omit it and the CLI fetches the catalog from the main branch.
REPO_ROOT = $$(git rev-parse --show-toplevel)
DEMO_TEAM ?= payments
DEMO_APP  ?= checkout-api
DEMO_PATH ?= go-service-postgres

demo-onboard-team:
	cd 2-idp-scaffolder/golang && go run . onboard-team \
	  --output-root  "$(REPO_ROOT)/3-tenant-workloads" \
	  --catalog-root "$(REPO_ROOT)/1-platform-catalog" \
	  --team-name    "$(DEMO_TEAM)"

demo-add-service:
	cd 2-idp-scaffolder/golang && go run . add-service \
	  --output-root  "$(REPO_ROOT)/3-tenant-workloads" \
	  --catalog-root "$(REPO_ROOT)/1-platform-catalog" \
	  --team-name    "$(DEMO_TEAM)" \
	  --app-name     "$(DEMO_APP)" \
	  --golden-path  "$(DEMO_PATH)"


