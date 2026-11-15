# WARP Tray

A lightweight Windows system tray app for controlling Cloudflare WARP. Click the cloud icon to connect or disconnect, without opening a window.

## Setup

1. Download and install Cloudflare WARP from [one.one.one.one](https://one.one.one.one).
2. Launch Cloudflare WARP once and complete its setup.
3. Fully exit the Cloudflare WARP app and disable its automatic startup. Keep the Cloudflare WARP background service enabled: WARP Tray needs it to manage the connection.
4. Download `warp-tray.exe` from this repository's **Nightly Release** and move it to:

   ```text
   %PROGRAMDATA%\Microsoft\Windows\Start Menu\Programs\Startup
   ```

   You can paste this path into the File Explorer address bar. Administrator permission may be required because this startup folder applies to all users.

5. Open `warp-tray.exe` to start it now. From then on, Windows will start it automatically when you sign in.

## Usage

- **Left-click the cloud:** connect or disconnect WARP.
- **Green check mark:** connected.
- **Red X:** disconnected.
- **Yellow exclamation mark:** connecting, disconnecting, checking the connection, or status unavailable.
- **Hover over the icon:** view the current status.
- **Right-click → Exit:** close WARP Tray without disconnecting WARP.

If the icon is hidden, open the system tray overflow menu (`^`) and drag it onto the taskbar notification area.
