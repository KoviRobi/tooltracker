# ![Tooltracker logo: T/question-mark ligature in front of the purple Carallon planet](artwork/logo.svg) Tool tracker

Prototype to make tracking tools easy, by labelling them with QR codes. Only
requires a phone with QR scanning and emailing capabilities. Zero install for users.

![Tooltracker flow, grab object, scan QR code, done](artwork/cover.svg)

The usual shared spreadsheet tool trackers are trivial, but require discipline
and knowledge:

1. discipline because you have to remember to update the tool, that you just
   want to use. If you use it first, then you will almost certainly forget. And
   shared responsibility, if someone says they took a tool, while you are
   working nearby, they might assume you will update the tracker, but you might
   be busy and assume they will update the tracker;
2. knowledge of where the tracker is, this is often implicit, and can be
   different for different tools.

The aim with this, is that if I make tracking as simple as possible, it is more
likely to be used properly. And have as minimal dependencies as possible,
ideally only a smartphone, without any special program set up. E-mail and QR
codes work nicely here.

## Installation

This is a simple go app, using ODBC for the database. You can install it with

```sh
go install github.com/KoviRobi/tooltracker/cmd/tooltracker@latest
```

See `tooltracker --help` for options.

See section [Deploying](#deploying) for more details.

## Usage

To check which item was last seen by whom, navigate to
[http://〈deployed.host〉/〈http-prefix〉/](#).

To add items, simply go to
[http://〈deployed.host〉/〈http-prefix〉/tool?name=〈name〉](#) to print a QR
code label, stick it onto the object you want to track. Whenever someone scans
it on their phone, it opens up an email saying they have borrowed the tool.
Replace 〈deployed-host〉 and 〈http-prefix〉 based on the configuration, and
〈name〉 based on what you want to call the tool. The first e-mail adds the
item to the database, but you can also add a picture and description on the
item's page.

## Authentication

There isn't a password style authentication, instead what you can do is use the
`-dkim mycompany.com` flag, which will require (initially) all emails to be
sent from `*@mycompany.com`.

Because this would stop users being able to use their phone if they don't have
the work e-mail set up, there is an escape valve: if they send an e-mail from a
work account along the lines of

```email
From: user1@mycompany.com
To: tooltracker@mycompany.com
Subject: Alias user1@personal.com user1234@other.com

Some alias description
```

Then this allows them to delegate `user1@personal.com` or `user1234@other.com`
as emails which can also send emails. The alias will initially apply to all
three, they can customize it.

## Deploying

To deploy, you should set up the go program somewhere it can receive mail on
port 25, and also somewhere where it can host webpages, presumably behind a
company VPN to not have the tracker website open to all.

For a fun way to test/introduce this, there are some UV mapped origami cubes in
[./misc](./misc). You will want to change the QR codes for your own deployment.
The idea is to hide the cubes somewhere, record a hint for their location in
the tracker. Then when people find a cube, they got some sweets as a
reward/incentive, and hide it for the next person, using their phone to give a
hint.

### NixOS (AWS)

There is an example NixOS system in
[./example-nixos-system.nix](./example-nixos-system.nix), you can customize
it and deploy it. I used it to test on an AWS EC2 instance:

1. Provision a machine with the latest NixOS AMI, see [https://nixos.github.io/amis/](https://nixos.github.io/amis/)
2. Edit your SSH configuration to be able to SSH as `aws` (for convenience)

   ```ssh_config
   Host aws
     User root
     IdentityFile ~/.ssh/id_ed25519.pub
     HostName 18.175.197.55
     UserKnownHostsFile /dev/null
     StrictHostKeyChecking accept-new
   ```

3. Modify the configuration in [./example-nixos-system](./example-nixos-system)
4. Generate a hardware config using

   ```sh
   ssh aws nixos-generate-config --show-hardware-config \
       >./example-nixos-system/hardware-configuration.nix
   git add ./example-nixos-system/hardware-configuration.nix
   git commit -m 'Add hardware-configuration for deployment TODO'
   ```

5. Do the deploy (replace hostname as appropriate):

   ```sh
   deploy '.#aws' --ssh-user root
   ```

### NixOS (LXC)

1. Modify the configuration in [./example-nixos-system](./example-nixos-system)
2. Build an LXC image with

   ```sh
   nix build '.#nixosConfigurations.example.config.formats.lxc' -o lxc-image
   nix build '.#nixosConfigurations.example.config.formats.lxc-metadata' -o lxc-metadata
   lxc image import ./lxc-image/nixos-*.tar.xz ./lxc-metadata/nixos-*.tar.xz
   ```

### Cloud-init/Ubuntu/Ansible

To build a VM, first create the `cidata.iso` for cloud-init, then boot the
Ubuntu `cloudimg` with that CD attached. These instructions assume
libvirt/qemu, but other VM methods should work, as long as you attach the ISO
generated by `genisoimage` in step 2.

- The base image to the `qemu-img create` is from
  [http://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img](http://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img)

    1. Set up your SSH to make connecting easy, add the following to your `~/.ssh/config`:

       > 🛈  **Note:** **The `cat <<ROF` and `EOF` lines are not part of**
       > **the file, they indicate the file name**

       ```sh
       cat <<EOF >>~/.ssh/config
       Host tooltracker-ansible
           User root
           HostName 10.1.0.2
           UserKnownHostsFile /dev/null
           StrictHostKeyChecking accept-new
       EOF
       ```

    2. Create cloud-init NoCloud configuration ISO (see
       <https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html>)

        Don't forget to make sure the SSH key is correct in user-data!

       ```sh
       cat >meta-data <<EOF
       instance-id: tooltracker-ansible
       local-hostname: tooltracker-ansible
       EOF
       ```

       > 🛈  **Note:** **This uses your ED25519 SSH key to access, make sure**
       > **it exists, or use a different key as required.**

       ```sh
       cat >user-data <<EOF
       #cloud-config
       users:
       - name: root
         ssh_authorized_keys:
         - $(cat ~/.ssh/id_ed25519.pub)
       EOF
       ```

       Fixed IP/MAC, the MAC should match the one in the `virt-install`
       command, or your VM. I am using `10.1.0.2/24` for the VM and
       `10.1.0.1/24` for the host machine.

       ```sh
       cat >network-config <<EOF
       version: 2
       ethernets:
         enp1s0:
           match:
             macaddress: "52:54:00:b2:f8:31"
           set-name: "enp1s0"
           nameservers:
             addresses: ["1.1.1.1"]
           addresses: ["10.1.0.2/24"]
           gateway4: "10.1.0.1"
           dhcp4: false
       EOF
       ```

        Finally, generate the ISO image. This is using `genisoimage` from
        `cdrkit`, but other ISO generators should work, as long as the volume
        name is `CIDATA`:

       ```sh
       genisoimage \
           -output cidata.iso \
           -V cidata \
           -r \
           -J user-data meta-data network-config
       ```

    3. Create an image for just our VM -- this uses copy-on-write (COW) to
       base it off the Ubuntu cloudimg downloaded from
       <http://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img>

       ```sh
       qemu-img create \
           -f qcow2 \
           -b ~/Downloads/ubuntu-24.04-server-cloudimg-amd64.img -F qcow2 \
           tooltracker-ansible.img 10G
       ```

       > 🛈  **Note:** **I have a bridge `virbr0` with IP `10.1.0.1` and
       > dnsmasq** **(NetworkManager "Shared to others"), sharing normal
       > internet.**

       > 🛈  **Note:** **This creates a transient VM (see `--transient`), and destroys it**
       > **when the VM exits (see the `virsh` command).**

       ```sh
       virt-install \
           --connect 'qemu:///system' \
           --name=tooltracker-ansible \
           --ram=2048 \
           --vcpus=2 \
           --import \
           --disk path=tooltracker-ansible.img,format=qcow2 \
           --disk path=cidata.iso,device=cdrom \
           --os-variant=ubuntu24.04 \
           --network bridge=virbr0,model=virtio,mac=52:54:00:b2:f8:31 \
           --autoconsole text \
           --transient; \
       virsh --connect 'qemu:///system' destroy tooltracker-ansible
       ```

- Once you have a VM, you can provision it using Ansible, for example:

  ```sh
  ansible-playbook -i inventory.yaml tooltracker.yaml
  ```
