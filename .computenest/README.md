# 代码仓库结构

文档目录说明：
```
├── README.md                   - 服务说明和服务构建说明
├── docs                        - 服务文档相关文件
│   └── index.md
├── resources                   - 服务资源文件
│   ├── icons
│   │   └── logo.png            - 服务logo
├── ros_templates               - 服务ROS模板，可以有多个
│   └── template.yaml           - 示例ROS模板
├── config.yaml                 - 服务配置文件
└── code_generation             - 代码生成相关，包括jinja2模板、jinja2变量值文件、jinja2变量定义文件
    ├── ros_templates           - 用来生成ROS模板的jinja2模板，可以有多个
    │   └── template.yaml.j2    - 用于生成ecs部署架构的模板
    ├── config.yaml.j2          - 用来生成config.yaml的jinja2模板
    ├── variables.yaml          - 保存jinja2模板变量值的文件
    └── variable_meta.yaml      - 定义variable格式的文件，用来在控制台渲染变量输入表单
```