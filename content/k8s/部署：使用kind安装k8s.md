+++
date = '2024-11-22T17:35:53+08:00'
title = "部署：使用kind安装k8s"
tags = ["kubernetes","k8s"]
categories = ["kubernetes","k8s"]
+++
本文记录了使用 kind 搭建一个 Kubernetes 集群的过程，适合学习、测试、小型自建集群.
使用机器是wsl2 Ubuntu2204

## 1. 安装kind
在开始之前，请确保你的本地机器上已经安装了 Docker 并且 Docker 守护进程正在运行。
```bash
# 建议去 GitHub Releases 页面查看最新版本号替换 v0.22.0
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.22.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```

## 2. 创建k8s集群
```bash
kind create cluster --name test-cluster
```
执行后，Kind 会自动拉取节点镜像并启动容器，同时会自动将 kubectl 的 context 切换到新创建的集群。

## 3. 进阶:创建多节点集群
在实际生产中，我们通常有多个 Worker 节点。Kind 允许你通过 YAML 配置文件来定义集群的拓扑结构。
创建一个名为 kind-config.yaml 的文件，写入以下内容（1个控制平面 + 2个工作节点）：
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
```
使用该配置文件创建集群：
```bash
kind create cluster --name multi-node-cluster --config kind-config.yaml
```

## 4. 高级配置：端口映射与目录挂载
在本地开发时，我们经常需要将宿主机的目录挂载到 K8s 节点中，或者将 NodePort 映射到宿主机以便通过浏览器访问服务。Kind 的配置文件同样支持这些高级特性。
创建一个 advanced-config.yaml：
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  # 端口映射：将容器的 30080 端口映射到宿主机的 30080 端口
  extraPortMappings:
  - containerPort: 30080
    hostPort: 30080
    protocol: TCP
  # 目录挂载：将本地代码目录挂载到节点中
  extraMounts:
  - hostPath: /Users/yourname/my-project
    containerPath: /files
- role: worker
```

## 5. 配置kubectl
Kind 只负责创建集群（会配置好 kubeconfig），后续操作集群的话需要手动安装 kubectl[2]。
如未安装 kubectl，可参考：
```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/
```
验证集群连接：
```bash
kubectl cluster-info --context kind-test-cluster
kubectl get nodes
```

## 6. 痛点解决：加载本地 Docker 镜像
场景：你在本地修改了代码，用 docker build 打包了一个新镜像 my-app:v2。由于 Kind 的节点是独立的 Docker 容器，它无法直接访问你宿主机的本地镜像，去远端仓库 push/pull 又太慢。
解决方案：Kind 提供了 load 命令，可以将本地镜像直接塞进集群节点中。
```bash
# 1. 本地构建镜像
docker build -t my-app:v2 .

# 2. 将镜像加载到 kind 集群中
kind load docker-image my-app:v2 --name multi-node-cluster
```
注意：在编写 K8s Deployment YAML 时，请务必将 imagePullPolicy 设置为 IfNotPresent 或 Never，否则 K8s 依然会尝试去 Docker Hub 拉取镜像而导致 ImagePullBackOff 错误。


## 7. 常用操作
删除集群：
```bash
kind delete cluster --name test-cluster
```
查看集群列表：
```bash
kind get clusters
```
切换集群：
```bash
kubectl config use-context kind-${CLUSTER_NAME}
```