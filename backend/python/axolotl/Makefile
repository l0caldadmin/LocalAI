.PHONY: axolotl
axolotl:
	bash install.sh

.PHONY: run
run: axolotl
	@echo "Running axolotl..."
	bash run.sh
	@echo "axolotl run."

.PHONY: protogen-clean
protogen-clean:
	$(RM) backend_pb2_grpc.py backend_pb2.py

.PHONY: clean
clean: protogen-clean
	rm -rf venv __pycache__
