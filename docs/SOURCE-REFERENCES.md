# Provider Source References

实现任何 Provider Adapter 前必须重新核对当前官方文档/源码，不得仅依赖本文件。

## LXDAPI
- Repository: https://github.com/xkatld/lxdapi-web-server
- Wiki: https://github.com/xkatld/lxdapi-web-server/wiki

## Canonical LXD Direct API
- Instance creation: https://github.com/canonical/lxd/blob/main/doc/howto/instances_create.md
- Go client interfaces and operation contract: https://github.com/canonical/lxd/blob/main/client/interfaces.go
- REST query client source: https://github.com/canonical/lxd/blob/main/lxc/query.go
- Projects: https://documentation.ubuntu.com/lxd/latest/howto/projects_create/
- Current documentation (authentication, instances, operations): https://documentation.ubuntu.com/lxd/latest/
- Last verified: 2026-09-23

## CLICD
- Documentation: https://cli.cd/guide/introduction
- Site/API docs: https://cli.cd/

## Runman Agent
- Repository: https://github.com/narwhal-cloud/runman-agent
- Protocol source should be read from the repository before implementing Gateway.

## Rule
第三方项目发生 API/协议变化时，只修改对应 Adapter/Gateway；Business Core、Domain Model、Operation Contract 不随意变化。
