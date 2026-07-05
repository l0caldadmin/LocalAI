#!/usr/bin/env python3
"""
Axolotl fine-tuning backend for LocalAI.

Wraps the Axolotl training CLI. It takes structured `extra_options` from the Go API,
constructs an Axolotl YAML configuration, and executes the training run, streaming
progress back to LocalAI.
"""
import argparse
import json
import os
import queue
import subprocess
import signal
import sys
import threading
import traceback
import uuid
import yaml
from concurrent import futures

import grpc
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'common'))
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'common'))
from grpc_auth import get_auth_interceptors

import backend_pb2
import backend_pb2_grpc

_ONE_DAY_IN_SECONDS = 60 * 60 * 24
MAX_WORKERS = int(os.environ.get('PYTHON_GRPC_MAX_WORKERS', '4'))


class Job:
    def __init__(self, job_id):
        self.job_id = job_id
        self.progress_queue = queue.Queue()
        self.completed = False
        self.error = None
        self._cancel_event = threading.Event()

    def put_progress(self, update):
        self.progress_queue.put(update)

    def cancel(self):
        self._cancel_event.set()


class BackendServicer(backend_pb2_grpc.BackendServicer):
    def __init__(self):
        self.jobs = {}
        self.jobs_lock = threading.Lock()

    def StartFineTune(self, request, context):
        job_id = str(uuid.uuid4())
        job = Job(job_id)
        
        with self.jobs_lock:
            self.jobs[job_id] = job

        # Start the background worker
        thread = threading.Thread(target=self._do_train, args=(request, job))
        thread.daemon = True
        thread.start()

        return backend_pb2.Result(success=True, message=job_id)

    def _do_train(self, request, job):
        try:
            extra = {}
            if request.extra_options:
                try:
                    extra = json.loads(request.extra_options)
                except Exception as e:
                    print(f"Warning: failed to parse extra_options as JSON: {e}", file=sys.stderr)

            # Construct the Axolotl configuration dictionary
            config = {
                "base_model": request.model,
                "sequence_len": int(extra.get("max_seq_length", 512)),
                "sample_packing": extra.get("sample_packing", True),
                "flash_attention": extra.get("flash_attention", False),
                "learning_rate": float(extra.get("learning_rate", 2e-5)),
                "num_epochs": int(extra.get("num_epochs", 3)),
                "micro_batch_size": int(extra.get("batch_size", 2)),
                "gradient_accumulation_steps": int(extra.get("gradient_accumulation_steps", 1)),
                "output_dir": request.output_dir,
                "val_set_size": 0,
            }

            if extra.get("dataset"):
                # E.g. {"path": "tatsu-lab/alpaca", "type": "alpaca"}
                config["datasets"] = [{"path": extra["dataset"], "type": extra.get("dataset_type", "alpaca")}]

            if request.training_type == "lora":
                config["adapter"] = "lora"
                config["lora_r"] = int(extra.get("lora_r", 8))
                config["lora_alpha"] = int(extra.get("lora_alpha", 16))
                config["lora_dropout"] = float(extra.get("lora_dropout", 0.05))
                config["lora_target_modules"] = [
                    m.strip() for m in extra.get("lora_target_modules", "q_proj,v_proj").split(",")
                ]

            # Write config to a temporary file
            config_path = os.path.join(request.output_dir, "axolotl_config.yml")
            os.makedirs(request.output_dir, exist_ok=True)
            with open(config_path, "w") as f:
                yaml.dump(config, f)

            # Launch Axolotl
            cmd = [sys.executable, "-m", "axolotl.cli.train", config_path]
            process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)

            while True:
                if job._cancel_event.is_set():
                    process.terminate()
                    break

                line = process.stdout.readline()
                if not line and process.poll() is not None:
                    break
                
                if line:
                    sys.stdout.write(line)
                    sys.stdout.flush()
                    # We could parse the line here to extract loss/steps for the progress bar
                    # For now we just stream a status update to keep the connection alive
                    job.put_progress(backend_pb2.FineTuneProgressUpdate(
                        job_id=job.job_id, status="training", message=line.strip(),
                    ))

            retcode = process.wait()
            if retcode == 0:
                job.completed = True
                job.put_progress(backend_pb2.FineTuneProgressUpdate(
                    job_id=job.job_id, status="completed", message="Training completed",
                    progress_percent=100.0,
                ))
            elif not job._cancel_event.is_set():
                raise RuntimeError(f"Axolotl process exited with code {retcode}")

        except Exception as exc:
            tb = traceback.format_exc()
            traceback.print_exc()
            job.completed = True
            job.put_progress(backend_pb2.FineTuneProgressUpdate(
                job_id=job.job_id, status="failed", message=f"Training failed: {exc}\n\nTraceback:\n{tb}",
            ))
        finally:
            job.put_progress(None)

    def FineTuneProgress(self, request, context):
        with self.jobs_lock:
            job = self.jobs.get(request.job_id)
            
        if not job:
            yield backend_pb2.FineTuneProgressUpdate(job_id=request.job_id, status="failed", message="Job not found")
            return

        while True:
            update = job.progress_queue.get()
            if update is None:
                break
            yield update

        if job.error:
            yield backend_pb2.FineTuneProgressUpdate(job_id=request.job_id, status="failed", message=job.error)

    def StopFineTune(self, request, context):
        with self.jobs_lock:
            job = self.jobs.get(request.job_id)
        if job:
            job.cancel()
            return backend_pb2.Result(success=True, message="Job cancelled")
        return backend_pb2.Result(success=False, message="Job not found")


def serve(address):
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=MAX_WORKERS),
        interceptors=get_auth_interceptors(),
        options=[
            ('grpc.max_send_message_length', -1),
            ('grpc.max_receive_message_length', -1),
        ]
    )
    backend_pb2_grpc.add_BackendServicer_to_server(BackendServicer(), server)
    server.add_insecure_port(address)
    server.start()
    print(f"Server started. Listening on {address}")

    def sig_handler(sig, frame):
        print(f"Received signal {sig}, shutting down...")
        server.stop(0)
        sys.exit(0)

    signal.signal(signal.SIGINT, sig_handler)
    signal.signal(signal.SIGTERM, sig_handler)
    
    server.wait_for_termination()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="LocalAI Axolotl Backend")
    parser.add_argument("--addr", type=str, default="127.0.0.1:50051", help="Address to listen on")
    args = parser.parse_args()
    serve(args.addr)
