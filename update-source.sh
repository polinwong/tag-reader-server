sudo systemctl stop model-server.service
read -p "Press [Enter] key when you uploaded binary file..."
sudo setcap 'cap_net_bind_service=+ep' /home/marvel/model-sourceserver/sourceserver-linux
sudo systemctl start model-server.service
