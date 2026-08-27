+++
date = '2024-11-12T20:24:34+08:00'
title = "部署：使用kubeadm安装k8s"
tags = ["kubernetes","k8s"]
categories = ["kubernetes","k8s"]
+++

本文记录了使用 kubeadm 从头搭建一个使用 containerd 作为容器运行时的 Kubernetes 集群的过程，适合学习、测试、小型自建集群
本文用到的机器如下：
| 主机名 | 系统版本 | 配置 | IP | 角色 | 
|:-------:|:--------:|:-------:|:-------:|:-------:|
| k8s-master | Ubuntu | 2C4G | 192.168.5.131 | master |
| k8s-node01 | Ubuntu | 2C4G | 192.168.5.132 | worker |
| k8s-node02 | Ubuntu | 2C4G | 192.168.5.133 | worker |

## 1. 部署设置
这一步的操作需要在所有节点上完成

### 1.1 设置主机名
在不同的节点上分别执行：
```bash
sudo hostnamectl set-hostname k8s-master
# 或
sudo hostnamectl set-hostname k8s-node01
# 或
sudo hostnamectl set-hostname k8s-node02
```
可选：在所有节点配置 /etc/hosts，方便节点之间通过主机名访问：
```bash
sudo tee -a /etc/hosts <<EOF
192.168.5.131 k8s-master
192.168.5.132 k8s-node01
192.168.5.133 k8s-node02
EOF
```

### 1.2 关闭swap
默认情况下，kubelet 检测到 swap 后会启动失败，除非你显式配置 kubelet 允许 swap。因此常见安装方式是直接关闭 swap。
```bash
sudo swapoff -a
sudo sed -i.bak '/ swap / s/^/#/' /etc/fstab
```
检查：
```bash
free -h
```
如果 Swap 为 0，说明已关闭。


### 1.3 配置内核参数
Kubernetes 网络插件通常需要主机允许 IPv4 转发。
```bash
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF

sudo sysctl --system
```
检查：
```bash
sysctl net.ipv4.ip_forward
```

看到如下结果即可：
```bash
net.ipv4.ip_forward = 1
```

### 1.4 安装containerd
Kubernetes 需要在每个节点安装容器运行时。Kubernetes v1.36 要求容器运行时符合 CRI；containerd 是常见选择之一。
```bash
sudo apt-get update
sudo apt-get install -y containerd
```
生成默认配置文件：
```bash
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
```
将 containerd 的 cgroup driver 配置为 systemd。Kubernetes 官方文档强调，kubelet 和容器运行时必须使用一致的 cgroup driver；在使用 systemd 的 Linux 发行版上，推荐使用 systemd cgroup driver。

```bash
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
```
重启并设置开机自启：
```bash
sudo systemctl restart containerd
sudo systemctl enable containerd
sudo systemctl status containerd
```

### 1.5 安装kubeadm、kubelet、kubectl
kubeadm 用来初始化集群，kubelet 负责在每个节点上运行 Pod，kubectl 是访问 Kubernetes API 的命令行工具。
```bash
sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl gpg
```
添加 Kubernetes 软件源签名：
```bash
sudo mkdir -p -m 755 /etc/apt/keyrings

curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.36/deb/Release.key \
  | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
```
添加 Kubernetes v1.36 软件源：
```bash

echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.36/deb/ /' \
  | sudo tee /etc/apt/sources.list.d/kubernetes.list
```

安装组件并锁定版本：
```bash
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
```
启动 kubelet：
```bash
sudo systemctl enable --now kubelet
```
此时 kubelet 可能暂时处于重启状态，这是正常的；等控制平面初始化完成后，它会正常工作。

### 1.6 控制平台节点：初始化 Kubernetes 集群
在 k8s-master 上执行：
```bash
sudo kubeadm init \
  --apiserver-advertise-address=192.168.5.131 \
  --pod-network-cidr=10.244.0.0/16
```
这里的两个关键参数：

--apiserver-advertise-address 指定 API Server 对外通告的地址，一般填写控制平面节点的内网 IP。

--pod-network-cidr 指定 Pod 网络地址段。本文后面使用 Flannel，因此采用 Flannel 默认常见的 10.244.0.0/16。Flannel 官方说明中也提到，如果使用的 Pod CIDR 不是 10.244.0.0/16，需要修改对应 manifest。

初始化成功后，终端会输出类似内容：
```bash
Your Kubernetes control-plane has initialized successfully!
```
同时会给出一条 kubeadm join ... 命令，后面工作节点加入集群时要用到。

### 1.7 控制平面节点：配置 kubectl
普通用户要使用 kubectl，需要复制 kubeconfig：
```bash
mkdir -p $HOME/.kube

sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config

sudo chown $(id -u):$(id -g) $HOME/.kube/config
```

检查节点：
```bash
kubectl get nodes
```
此时可能看到：
```bash
NAME         STATUS     ROLES           AGE   VERSION
k8s-master   NotReady   control-plane   1m    v1.36.x
```
NotReady 是因为还没有安装 CNI 网络插件

### 1.8 控制平面节点：安装 Pod 网络插件
Kubernetes 集群必须安装基于 CNI 的 Pod 网络插件，否则 Pod 之间无法正常通信。
本文使用 Flannel：
```bash
kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
```
等待组件启动：
```bash
kubectl get pods -A
```
当看到 coredns、kube-flannel 等 Pod 都变成 Running，再检查节点：
```bash
kubectl get nodes
```
预期结果：
```bash
NAME         STATUS   ROLES           AGE   VERSION
k8s-master   Ready    control-plane   5m    v1.36.x
```

### 1.9 工作节点：加入集群
之前提到控制节点init成功后会给出一条 kubeadm join ... 命令。执行命令即可。
```bash
sudo kubeadm join 192.168.56.10:6443 \
  --token xxxxxx.xxxxxxxxxxxxxxxx \
  --discovery-token-ca-cert-hash sha256:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```
如果忘记了 join 命令，可以在控制平面节点重新生成：
```bash
kubeadm token create --print-join-command
```
加入完成后，在控制平面节点检查：
```bash
kubectl get nodes
```
预期结果：
```bash
NAME         STATUS   ROLES           AGE   VERSION
k8s-master   Ready    control-plane   10m   v1.36.x
k8s-node01   Ready    <none>          2m    v1.36.x
k8s-node02   Ready    <none>          2m    v1.36.x
```

## 2. 测试
创建一个 Nginx Deployment：
```bash
kubectl create deployment nginx --image=nginx:latest
```
暴露服务：
```bash
kubectl expose deployment nginx --port=80 --type=NodePort
```

查看 Pod：
```bash
kubectl get pods -o wide
```

查看 Service：
```bash
kubectl get svc nginx
```

输出类似：
```bash
NAME    TYPE       CLUSTER-IP      EXTERNAL-IP   PORT(S)        AGE
nginx   NodePort   10.96.xxx.xxx   <none>        80:30xxx/TCP   1m
```
然后访问任意节点 IP 加 NodePort：
```bash
curl http://192.168.5.131:30xxx
```
如果返回 Nginx 欢迎页，说明集群已经可以正常调度和访问应用。
