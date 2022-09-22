package constants

// CR metadata
const (
	CrKind       = "SSANginx"
	FieldManager = "ssanginx-fieldmanager"
	Namespace    = "ssa-nginx-controller-system"
)

// The field corresponding to the index specified in FieldIndexer.
// There is no problem even if the path is different from the actual index path.
const (
	IndexOwnerKey = ".metadata.ownerReference.name"
)

// Container info
const (
	InitConatainerName  = "init"
	InitConatainerImage = "alpine"
	CompareImageName    = "nginx"
)

// container command
const (
	InitCommand = `cat << EOT > /tmp/run-nginx.sh
apt-get update
apt-get install inotify-tools -y
nginx
EOT
chmod 500 /tmp/run-nginx.sh
cat << EOT > /tmp/auto-reload-nginx.sh
oldcksum=\` + "`" + `cksum /etc/nginx/conf.d/default.conf\` + "`" + `
inotifywait -e modify,move,create,delete -mr --timefmt '%d/%m/%y %H:%M' --format '%T' /etc/nginx/conf.d/ | \
while read date time; do
  newcksum=\` + "`" + `cksum /etc/nginx/conf.d/default.conf\` + "`" + `
  if [ "\${newcksum}" != "\${oldcksum}" ]; then
    echo "At \${time} on \${date}, config file update detected."
    oldcksum=\${newcksum}
    service nginx restart
  fi
done
EOT
chmod 500 /tmp/auto-reload-nginx.sh
`
	ContainerCommand = `/tmp/run-nginx.sh && /tmp/auto-reload-nginx.sh`
)

// volume names
const (
	ConfVolumeName     = "conf"
	EmptyDirVolumeName = "nginx-reload"
	IndexVolumeName    = "index"
)

// configmap volume key
const (
	ConfVolumeKeyPath = "default.conf"
)

// volume mountpath
const (
	ConfVolumeMountPath     = "/etc/nginx/conf.d/"
	EmptyDirVolumeMountPath = "/tmp/"
	IndexVolumeMountPath    = "/usr/share/nginx/html/"
)

// Secret Info
const (
	IngressSecretName = "ca-secret"
	ClientSecretName  = "cli-secret"
)

// Ingress Info
const (
	IngressClassName = "nginx"
)
