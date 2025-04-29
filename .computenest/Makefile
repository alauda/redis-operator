all: install gen

install:
	pip install computenest-cli==1.2.9

gen:
	computenest-cli generate --file_path=code_generation/config.yaml.j2 --parameter_path=code_generation/variables.yaml --output_path=config.yaml
	computenest-cli generate --file_path=code_generation/ros_templates/template.yaml.j2 --parameter_path=code_generation/variables.yaml --output_path=ros_templates/template.yaml
