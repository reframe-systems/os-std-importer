FROM public.ecr.aws/lambda/provided:al2023

# Install git (required for all git shell-outs in the importer)
RUN dnf install -y git && dnf clean all

# Copy the pre-built Linux binary; Lambda's provided runtime requires the
# entrypoint to be named 'bootstrap'
COPY os-std-importer-linux /var/runtime/bootstrap
RUN chmod +x /var/runtime/bootstrap

CMD ["bootstrap"]
