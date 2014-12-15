{% if grains.network_mode is defined and grains.network_mode == 'openvswitch' %}

openvswitch:
  pkg:
    - installed
  service.running:
    - enable: True

{% endif %}
