IMAGE=quay.io/utilitywarehouse/registry-browser

release:
	@sd "$(IMAGE):latest" "$(IMAGE):$(VERSION)" $$(rg -l -- $(IMAGE) manifests/)
	@git add -- manifests/
	@git commit -m "Release $(VERSION)"
	@sd "$(IMAGE):$(VERSION)" "$(IMAGE):latest" $$(rg -l -- "$(IMAGE)" manifests/)
	@git add -- manifests/
	@git commit -m "Clean up release $(VERSION)"
