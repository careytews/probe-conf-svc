
//
// Definition for VPN resources on Kubernetes.
//

// Import KSonnet library.
local k = import "ksonnet.beta.2/k.libsonnet";
local tnw = import "lib/tnw-common.libsonnet";

// Short-cuts to various objects in the KSonnet library.
local depl = k.extensions.v1beta1.deployment;
local container = depl.mixin.spec.template.spec.containersType;
local containerPort = container.portsType;
local mount = container.volumeMountsType;
local volume = depl.mixin.spec.template.spec.volumesType;
local resources = container.resourcesType;
local env = container.envType;
local svc = k.core.v1.service;
local svcPort = svc.mixin.spec.portsType;
local svcLabels = svc.mixin.metadata.labels;
local externalIp = svc.mixin.spec.loadBalancerIp;
local svcType = svc.mixin.spec.type;
local secretDisk = volume.mixin.secret;
local configMap = k.core.v1.configMap;

local pvcVol = volume.mixin.persistentVolumeClaim;
local pvc = k.core.v1.persistentVolumeClaim;
local sc = k.storage.v1.storageClass;

local probeConfigSvc(config) = {

    name: "probe-conf-svc",
    version:: import "version.jsonnet",
    namespace: config.namespace,
    images: [config.containerBase + "/probe-conf-svc:" + self.version],

    local ports = [
        containerPort.newNamed("probe-conf", 443)
    ],
    local volumeMounts = [
        mount.new("probe-conf-svc-creds", "/creds") + mount.readOnly(true),
        mount.new("probe-configuration-keys", "/keys"),
        mount.new("probe-conf-data", "/data")
    ],
    local containers = [
        container.new("probe-conf-svc", self.images[0]) +
            container.ports(ports) +
            container.volumeMounts(volumeMounts) +
            container.mixin.resources.limits({
                memory: "64M", cpu: "1.0"
            }) +
            container.mixin.resources.requests({
                memory: "64M", cpu: "0.05"
            })
    ],
    // Volumes - this invokes a secret containing the cert/key
    local volumes = [

        // probe-svc-creds private.json
        volume.name("probe-configuration-keys") +
            secretDisk.secretName("probe-configuration-keys"),

        // probe-svc-creds TLS keys
        volume.name("probe-conf-svc-creds") +
            secretDisk.secretName("probe-conf-svc-creds"),

        volume.name("probe-conf-data") + pvcVol.claimName("probe-conf-data")

    ],
    // Deployments
    deployments: [
        depl.new("probe-conf-svc", 1, containers,
                 {app: "probe-conf-svc", component: "access"}) +
            depl.mixin.spec.template.spec.volumes(volumes) +
            depl.mixin.metadata.namespace(self.namespace)
    ],
    // Ports used by the service.
    local servicePorts = [
        svcPort.newNamed("probe-conf", 443, 443) + svcPort.protocol("TCP")
    ],
    // Service
    services:: [

        svc.new("probe-conf", {app: "probe-conf-svc"}, servicePorts) +

           // Load-balancer and external IP address
           externalIp(config.addresses.probeConfSvc) + svcType("LoadBalancer") +

           // This traffic policy ensures observed IP addresses are the external
           // ones
           svc.mixin.spec.externalTrafficPolicy("Local") +

           // Label
           svcLabels({app: "probe-conf", component: "access"}) +
            
           svc.mixin.metadata.namespace(self.namespace)

    ],

    storageClasses:: [
        sc.new() + sc.mixin.metadata.name("probe-conf") +
            config.storageParams.hot +
            { reclaimPolicy: "Retain" } +
            sc.mixin.metadata.namespace(self.namespace)
    ],

    pvcs:: [
        tnw.pvc("probe-conf-data", "probe-conf", 25, self.namespace)
    ],

    diagram: [
	"probeconfsvc [label=\"probe configuration svc\"]"
    ],
    
    resources:
        if config.options.includeProbeConfSvc then
            self.deployments + self.services + self.storageClasses + self.pvcs
        else []

};

[probeConfigSvc]
