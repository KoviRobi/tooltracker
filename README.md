# Tool tracker
Prototype to make tracking tools easy, by labelling them with QR codes. Only
requires a phone with QR scanning and emailing capabilities.

The usual shared spreadsheet tool trackers are trivial, but require discipline and knowledge:
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

## Example deploy
In the flake.nix there is an example deployment I have used to test the
tooltracker out. It was installed on top of an AWS EC2 instance. To test out
for your own deployment:

1. Create an EC2 instance using the basic Amazon Linux image (it can be the
   free tier `t2.micro` or `t3.micro` depending on location, which is what I
   used, though due to RAM limitations the initial `nixos-install-tools` might
   get OOMed, that is okay, if you run it two/three times it should work [just
   due to the download parallelism, downloading multiple packages to RAM]).
2. Because private SSH keyfile isn't in this repo

   ```
   ssh-keygen -f example -t ed25519
   git add example.pub
   ```

   and set the `openssh.authorizedKeys.keyFiles` in flake.nix.
3. Create a nix key your machine signs derivations with, see
   https://nix.dev/manual/nix/2.18/command-ref/nix-store/generate-binary-cache-key
   but something along the lines of

   ```
   sudo mkdir /etc/secrets/nix
   sudo chmod 0700 /etc/secrets/nix
   sudo nix-store --generate-binary-cache-key mymachine /etc/secrets/nix/secret-key /etc/secrets/nix/secret-key.pub
   sudo cat /etc/secrets/nix/secret-key.pub
   ```

   and copy the public key into the `nix.settings.trusted-public-keys` in flake.nix
4. Install nix
   ```
   sh <(curl -L https://nixos.org/nix/install) --daemon
   # Restart shell for Nix to take effect
    exec $SHELL
   ```
   and add the public key from the previous step to the `extra-trusted-public-keys`,
   then run `sudo systemctl restart nix-daemon`.
   of `/etc/nix/nix.conf`.
5. Run nixos-generate-config to check the hardware settings are correct
   ```
   nix-env -f '<nixpkgs>' -iA nixos-install-tools
   ```
   might get OOM killed and need running again, but should eventually work.
   Because we are going to use deploy-rs this shouldn't be a problem for the
   system going onwards.
   ```
   nixos-generate-config --show-hardware-config
   ```
   update config in `flake.nix`, likely nothing actually needs changing but the
   `fileSystems` and `boot.initrd` bits are important to get correct. I found
   you don't want to enable any of the EFI boot settings as that overrides
   `boot.loader.grub.device = "nodev";`, and you want that.
6. Update your `~/.ssh/config` to be able to ssh to the node using `aws`, see
   the `deploy.nodes.aws` in `flake.nix`.
7. Update the domain and acme config (or just remove the acme/ACME and addSSL
   lines).
8. Commit the changes.
9. On your local machine, copy over the files to your AWS using
   ```
   deploy -s --dry-activate .
   ```
   this will print the system path in `path = "..."` form, copy that, for the
   next step. Incidentally, now is a really good time to make a snapshot from
   the disk.

10. Once you have taken a snapshot, it is time to switch over from Amazon Linux
    to NixOS:
    ```
    sudo touch /etc/NIXOS
    sudo touch /etc/NIXOS_LUSTRATE
    echo etc/nixos | sudo tee -a /etc/NIXOS_LUSTRATE
    echo home | sudo tee -a /etc/NIXOS_LUSTRATE

    sudo {path from deploy -s --dry-activate .}/bin/switch-to-configuration boot

    sudo mv /boot/grub2/grub.cfg /boot/grub2/grub.cfg.amzn
    sudo ln -s ../grub/grub.cfg /boot/grub2/grub.cfg

    sudo reboot
    ```
    Hopefully it should boot nicely.
