{% if grains['os_family'] == 'RedHat' %}
{% set environment_file = '/etc/sysconfig/kubelet-netbinder' %}
{% else %}
{% set environment_file = '/etc/default/kubelet-netbinder' %}
{% endif %}

sdn:
  cmd.wait:
    - name: /kubernetes-vagrant/network_closure.sh
    - watch:
      - pkg: docker-io
      - pkg: openvswitch

{{ environment_file}}:
  file.managed:
    - source: salt://kubelet-netbinder/default
    - template: jinja
    - user: root
    - group: root
    - mode: 644

/usr/local/bin/kubelet-netbinder:
  file.managed:
    - source: salt://kube-bins/kubelet-netbinder
    - user: root
    - group: root
    - mode: 755

{% if grains['os_family'] == 'RedHat' %}

/usr/lib/systemd/system/kubelet-netbinder.service:
  file.managed:
    - source: salt://kubelet-netbinder/kubelet-netbinder.service
    - user: root
    - group: root

{% else %}

/etc/init.d/kubelet-netbinder:
  file.managed:
    - source: salt://kubelet-netbinder/initd
    - user: root
    - group: root
    - mode: 755

{% endif %}

# The default here is that this file is blank.  If this is the case, the kubelet-netbinder
# won't be able to parse it as JSON and it'll not be able to publish events to
# the apiserver.  You'll see a single error line in the kubelet-netbinder start up file
# about this.
kubelet-netbinder:
  group.present:
    - system: True
  user.present:
    - system: True
    - gid_from_name: True
    - shell: /sbin/nologin
    - home: /var/lib/kubelet-netbinder
    - groups:
      - docker
      - kubelet-netbinder
    - require:
      - group: kubelet-netbinder
  service.running:
    - enable: True
    - watch:
      - file: /usr/local/bin/kubelet-netbinder
{% if grains['os_family'] != 'RedHat' %}
      - file: /etc/init.d/kubelet-netbinder
{% endif %}
      - file: /var/lib/kubelet/kubernetes_auth

