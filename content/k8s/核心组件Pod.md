+++
date = '2025-06-14T19:08:43+08:00'
title = "K8s系列之Pod"
tags = ["k8s"]
categories = ["k8s"]
+++

## 1. 为什么需要Pod?

在解释这个问题之前，我们需要对容器有一个认识：**容器的本质是进程。**



在一台宿主机中的操作系统中，进程并不是孤苦伶仃的运行，而是以进程组的方式来组织运行的，进程之间，可以共享文件、信号、数据内存、甚至部分代码，从而紧密协作成一个程序的职责。



而Kubernetes项目所做的，其实就是将“进程组”的概念映射到了容器技术中，并使其成为了这个云计算“操作系统”里的“一等公民”。



而容器是**单进程模型**，所以很多时候是多个容器来组织成应用本身。那在部署这个应用的时候，就需要将这多个容器部署在同一节点上，那该怎么做才能将多个容器准确的部署呢？



Kubernetes的解决方案：将一组容器放在一个Pod上进行组织，调度器按照Pod而非容器的资源需求进行计算的。自此就出现了一个很重要的概念：**Pod是Kubernetes里的原子调度单位**



像这样容器间的紧密协作，我们可以称为“超亲密关系”。



这些容器之间通常有以下特征：

- 需要部署在同一台机器上
- 会直接交换文件
- 通过 `localhost` 或 Socket 通信
- 有非常频繁的远程调用
- 需要共享某些 Linux Namespace
- 需要共享 Volume



例如：

- 应用容器和日志收集容器
- Web 服务器和应用部署包
- 主容器和 sidecar 辅助容器



但并不是所有有关系的应用都应该放进同一个 Pod。例如 PHP 应用和 MySQL 虽然有访问关系，但通常不应该放在同一个 Pod 中，因为它们不是“超亲密关系”。



Pod 不仅仅是为了解决调度问题，更重要的是它代表了一种 **容器设计模式**。



为了理解这一层含义，需要知道：**Pod 并不是一个真实存在的隔离边界，它只是一个逻辑概念。**



Kubernetes 真正操作的仍然是 Linux 的：

- Namespace
- Cgroups
- 容器
- 文件系统

Pod是一组共享了某些资源的容器， 其中资源主要是：

1. **共享同一个 Network Namespace**
2. **可以共享同一个 Volume**

那一个有A、B两个容器的Pod，不就是等同于一个容器（容器A）共享另外一个容器（容器B）的网络和Volume？



就好像通过命令`$ docker run --net=B --volumes-from=B --name=A image-A ...`来实现了。



但是这样启动的方式，就必须保证容器B比容器A先启动，这样的话，那Pod中的容器就不是对等关系了，成了拓扑关系了。



所以，在Kubernetes项目里，Pod的实现需要使用一个中间容器，这个容器叫作Infra容器。在这个Pod中，Infra容器永远都是第一个被创建的容器，而其他用户定义的容器，则通过Join Network Namespace的方式，与Infra容器关联在一起。

![](/k8s/images/k8s-infra.png)



这也就意味着，对于Pod里的容器A和容器B来说：

- 它们可以直接使用localhost进行通信；
- 它们看到的网络设备跟Infra容器看到的完全一样；
- 一个Pod只有一个IP地址，也就是这个Pod的Network Namespace对应的IP地址；
- 当然，其他的所有网络资源，都是一个Pod一份，并且被该Pod中的所有容器共享；
- Pod的生命周期只跟Infra容器一致，而与容器A和B无关。

## 2. Pod基本概念

Pod 扮演的是传统部署环境里“虚拟机”的角色。所以凡是调度、网络、存储，以及安全相关的属性，基本上是 Pod 级别的。

> 这些属性的共同特征是，它们描述的是“机器”这个整体。

**NodeSelector**：是一个供用户将 Pod 与 Node 进行绑定的字段。

```yaml
apiVersion: v1
kind: Pod
...
spec:
 nodeSelector:
   disktype: ssd
```

这样的一个配置，意味着这个Pod永远只能运行在携带了“disktype: ssd”标签（Label）的节点上；否则，它将调度失败。



**NodeName**：Pod具体调度到的节点。

一旦 Pod 的这个字段被赋值，Kubernetes 项目就会被认为这个 Pod 已经经过了调度，调度的结果就是赋值的节点名字。所以，这个字段一般由调度器负责设置，但用户也可以设置它来“骗过”调度器，当然这个做法一般是在测试或者调试的时候才会用到。

**HostAliases：定义了Pod的hosts文件（比如/etc/hosts）里的内容**，用法如下：

```yaml
apiVersion: v1
kind: Pod
...
spec:
  hostAliases:
  - ip: "10.1.2.3"
    hostnames:
    - "foo.remote"
    - "bar.remote"
...
```

在这个Pod的YAML文件中，我设置了一组IP和hostname的数据。这样，这个Pod启动后，/etc/hosts文件的内容将如下所示：

```vim
cat /etc/hosts
# Kubernetes-managed hosts file.
127.0.0.1 localhost
...
10.244.135.10 hostaliases-pod
10.1.2.3 foo.remote
10.1.2.3 bar.remote
```

其中最下面两行记录，就是我通过HostAliases字段为Pod设置的。

> 需要指出的是，在Kubernetes项目中，如果要设置hosts文件里的内容，一定要通过这种方法。否则，如果直接修改了hosts文件的话，在Pod被删除重建之后，kubelet会自动覆盖掉被修改的内容。

除了上述跟“机器”相关的配置外，你可能也会发现，**凡是跟容器的Linux Namespace相关的属性，也一定是Pod 级别的**。这个原因也很容易理解：Pod的设计，就是要让它里面的容器尽可能多地共享Linux Namespace，仅保留必要的隔离和限制能力。这样，Pod模拟出的效果，就跟虚拟机里程序间的关系非常类似了。

举个例子，在下面这个Pod的YAML文件中，我定义了shareProcessNamespace=true：

```
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  shareProcessNamespace: true
  containers:
  - name: nginx
    image: nginx
  - name: shell
    image: busybox
    stdin: true
    tty: true
```

这就意味着这个Pod里的容器要共享PID Namespace。

**Containers**

Pod 中最重要的字段当然是**Containers**了。

**首先，是 ImagePullPolicy 字段**，它定义了镜像拉取的策略，默认是 Always即每次创建 Pod 都重新拉取一次镜像。

**其次，是 Lifecycle 字段**，它定义的是 Container Lifecycle Hooks，在容器状态发生变化时触发一系列“钩子”。

```yaml
---
apiVersion: v1
kind: Pod
metadata:
  name: lifecycle-demo
spec:
  containers:
  - name: lifecycle-demo-container
    image: nginx
    lifecycle:
      postStart:
        exec:
          command: ["/bin/sh", "-c", "echo Hello from the postStart handler > /usr/share/message"]
      preStop:
        exec:
          command: ["/usr/sbin/nginx","-s","quit"]
```

postStart 容器启动后通过 echo 命令写入一段欢迎信息。

> 执行在 Docker 容器 ENTRYPOINT 执行之后，但不保证严格顺序，也就是说，在 postStart 启动时，ENTRYPOINT 有可能还没有结束。



preStop 容器被杀死之前（比如，收到了 SIGKILL 信号）调用 nginx 退出命令，优雅停止.

> 而需要明确的是，preStop 操作的执行，是同步的。所以，它会**阻塞**当前的容器杀死流程，直到这个 Hook 定义操作完成之后，才允许容器被杀死，这跟 postStart 不一样。
>
> 节点上的 `kubelet` 将等待**最多宽限期**（在 Pod 上指定，或从命令行传递；默认为 30 秒）以关闭容器，然后**强行终止进程**（使用 `SIGKILL`）。请注意，此宽限期包括执行 `preStop` 勾子的时间。

## 3. Pod 生命周期

Pod 遵循一个预定义的生命周期，起始于 `Pending` 阶段，如果至少 其中有一个主要容器正常启动，则进入 `Running`，之后取决于 Pod 中是否有容器以 失败状态结束而进入 `Succeeded` 或者 `Failed` 阶段。

在 Pod 运行期间，`kubelet` 能够重启容器以处理一些失效场景。 在 Pod 内部，Kubernetes 跟踪不同容器的状态并确定使 Pod 重新变得健康所需要采取的动作。

在 Kubernetes API 中，Pod 包含规约部分和实际状态部分。 Pod 对象的状态包含了一组 Pod 状况（Conditions）。 如果应用需要的话，你也可以向其中注入自定义的就绪性信息。

Pod 在其生命周期中只会被调度一次。 一旦 Pod 被调度（分派）到某个节点，Pod 会一直在该节点运行，直到 Pod 停止或者 被终止。



## 4. Pod阶段

Pod 生命周期的变化，主要体现在 Pod API 对象的 Status 部分，这是它除了 Metadata 和 Spec 之外的第三个重要字段。其中，pod.status.phase，就是 Pod 的当前状态，它有如下几种可能的情况：

1. **Pending**。这个状态意味着，Pod 的 YAML 文件已经提交给了 Kubernetes，API 对象已经被创建并保存在 Etcd 当中。但是，这个 Pod 里有些容器因为某种原因而不能被顺利创建。比如，调度不成功。
2. **Running**。这个状态下，Pod 已经调度成功，跟一个具体的节点绑定。它包含的容器都已经创建成功，并且至少有一个正在运行中。
3. **Succeeded**。这个状态意味着，Pod 里的所有容器都正常运行完毕，并且已经退出了。这种情况在运行一次性任务时最为常见。
4. **Failed**。这个状态下，Pod 里至少有一个容器以不正常的状态（非 0 的返回码）退出。这个状态的出现，意味着你得想办法 Debug 这个容器的应用，比如查看 Pod 的 Events 和日志。
5. **Unknown**。这是一个异常状态，意味着 Pod 的状态不能持续地被 kubelet 汇报给 kube-apiserver，这很有可能是主从节点（Master 和 Kubelet）间的通信出现了问题。

更进一步地，Pod 对象的 Status 字段，还可以再细分出一组 Conditions。这些细分状态的值包括：`PodScheduled`、`Ready`、`Initialized`，以及 `Unschedulable`。它们主要用于描述造成当前 Status 的具体原因是什么。

比如，Pod 当前的 Status 是 Pending，对应的 Condition 是 Unschedulable，这就意味着它的调度出现了问题。



## 5. 容器探针

### 5.1 健康检查

在 Kubernetes 中，你可以为 Pod 里的容器定义一个健康检查“探针”（Probe）。这样，kubelet 就会根据这个 Probe 的返回值决定这个容器的状态，而不是直接以容器镜像是否运行（来自 Docker 返回的信息）作为依据。

**这种机制，是生产环境中保证应用健康存活的重要手段**。

```yaml
---
apiVersion: v1
kind: Pod
metadata:
  labels:
    test: liveness
  name: test-liveness-exec
spec:
  containers:
  - name: liveness
    image: busybox
    args:
    - /bin/sh
    - -c
    - touch /tmp/healthy; sleep 30; rm -rf /tmp/healthy; sleep 600
    livenessProbe:
      exec:
        command:
        - cat
        - /tmp/healthy
      initialDelaySeconds: 5
      periodSeconds: 5
```

在这个 Pod 中，我们定义了一个有趣的容器。它在启动之后做的第一件事，就是在 /tmp 目录下创建了一个 healthy 文件，以此作为自己已经正常运行的标志。而 30 s 过后，它会把这个文件删除掉。

与此同时，我们定义了一个这样的 livenessProbe（健康检查）。它的类型是 exec，这意味着，它会在容器启动后，在容器里面执行一条我们指定的命令，比如：“cat /tmp/healthy”。这时，如果这个文件存在，这条命令的返回值就是 0，Pod 就会认为这个容器不仅已经启动，而且是健康的。这个健康检查，在容器启动 5 s 后开始执行（initialDelaySeconds: 5），每 5 s 执行一次（periodSeconds: 5）。

除了在容器中执行命令外，livenessProbe 也可以定义为发起 HTTP 或者 TCP 请求的方式，定义格式如下：

```yaml
...
livenessProbe:
     httpGet:
       path: /healthz
       port: 8080
       httpHeaders:
       - name: X-Custom-Header
         value: Awesome
       initialDelaySeconds: 3
       periodSeconds: 3
```

```yaml
...
    livenessProbe:
      tcpSocket:
        port: 8080
      initialDelaySeconds: 15
      periodSeconds: 20
```

所以，**你的 Pod 其实可以暴露一个健康检查 URL（比如 /healthz），或者直接让健康检查去检测应用的监听端口**。这两种配置方法，在 Web 服务类的应用中非常常用。



### 5.2 恢复机制

Kubernetes 里的 Pod 恢复机制，也叫 **restartPolicy**。它是 Pod 的 Spec 部分的一个标准字段（pod.spec.restartPolicy），默认值是 Always，即：任何时候这个容器发生了异常，它一定会被重新创建。

但一定要强调的是，Pod 的恢复过程，永远都是发生在当前节点上，而不会跑到别的节点上去。

> 事实上，一旦一个 Pod 与一个节点（Node）绑定，除非这个绑定发生了变化（pod.spec.node 字段被修改），否则它永远都不会离开这个节点。

这也就意味着，如果这个宿主机宕机了，这个 Pod 也不会主动迁移到其他节点上去。

> 而如果你想让 Pod 出现在其他的可用节点上，就必须使用 Deployment 这样的“控制器”来管理 Pod，哪怕你只需要一个 Pod 副本。

除了 Always，它还有 OnFailure 和 Never 两种情况：

- Always：在任何情况下，只要容器不在运行状态，就自动重启容器；
- OnFailure: 只在容器 异常时才自动重启容器；
- Never: 从来不重启容器。

在实际使用时，我们需要根据应用运行的特性，合理设置这三种恢复策略。

> 而如果你要关心这个容器退出后的上下文环境，比如容器退出后的日志、文件和目录，就需要将 restartPolicy 设置为 Never。

### 5.3 终止Pod

## 4. Pod 的终止

由于 Pod 所代表的是在集群中节点上运行的进程，当不再需要这些进程时允许其体面地 终止是很重要的。一般不应武断地使用 `KILL` 信号终止它们，导致这些进程没有机会 完成清理操作。

具体停止过程大致如下：

- 1）用户删除 Pod
- 2）Pod 进入 Terminating 状态;
- - 与此同时，k8s 会从对应的 service 上移除该 Pod 对应的 endpoint
  - 与此同时，针对有 preStop hook 的容器，kubelet 会调用每个容器的 preStop hook，假如 preStop hook 的运行时间超出了 grace period（默认30秒），kubelet 会发送 SIGTERM 并再等 2 秒;
  - 与此同时，针对没有 preStop hook 的容器，kubelet 发送 SIGTERM

- 3）grace period 超出之后，kubelet 发送 SIGKILL 干掉尚未退出的容器

kubelet 向runtime发送信号，最终runtime会将信号发送给容器中的主进程。

> 所以在程序中监听该信号可以实现优雅关闭。
