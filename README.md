# SoundShift

SoundShift is a desktop application that allows users to easily switch their default audio output devices and control their volume on Windows systems. It features a simple user interface built with the Fyne toolkit for Go, making audio device management accessible directly from the system tray.

## Features

- **Device Switching:** Quickly switch between audio output devices.
- **Volume Control:** Adjust the volume of your selected audio device with a convenient slider.
- **Configuration Settings:** Customize device visibility and application behavior through a configuration window.
- **System Tray Integration:** Access SoundShift from the system tray for convenience.

## Dependencies

SoundShift utilizes several packages to handle various functionalities:

- **Fyne:** For the graphical user interface.
- **go-ole/go-ole:** To interact with COM objects for managing audio devices.
- **go-vgo/robotgo, lxn/win:** For additional Windows API calls.
- **moutend/go-hook, moutend/go-wca:** For hooking into Windows Core Audio APIs.
- **winapi:** Custom Windows API wrapper used in the application.

## Building SoundShift

### Prerequisites

Ensure you have Go installed on your system. You can download it from [Go's official site](https://golang.org/dl/).

### Steps to Build

1. **Clone the Repository:**
    ```
    git clone https://github.com/yourusername/SoundShift.git
    cd SoundShift
    ```

2. **Build the Application:**
    ```
    go build -ldflags -H=windowsgui .
    ```

3. **Embed Icon (Optional - Requires ResourceHacker):**
    ```
    1. Download and install resource hacker if not already installed (https://angusj.com/resourcehacker/)
    2. Double click the Embed Icon.bat file located in the root directory of the source.

    The newly created executable will be embeded with "speaker.ico".
    ```

4. **Rename Application (Optional):**
    ```
    Rename the newly created executable to "SoundShift", rather that "soundshift". 
    This step should only be done AFTER embedding the icon, as the batch script has the word "soundshift.exe" hardcoded.
    ```

5. **Run the Application:**
    ```
    Simply double click the newly created executable and it will launch to your system tray.
    ```