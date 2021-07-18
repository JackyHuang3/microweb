# !/bin/bash
if [ "$1" = "microweb" ] ;then
  if [ "$2" != "example" ] ;then
    # echo "false:permission denied"
    echo "redirect:https://example.com?code="$2
  fi
fi
