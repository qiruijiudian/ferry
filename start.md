<h1>前端启动</h1>

    yarn run dev

<h1>后端启动</h1>

    go run main.go server -c=config/settings.dev.yml

<h1>docker 启动<h1>
    
    docker run -itd --name ferry -v /home/workorder/ferry/config:/opt/workflow/ferry/config -p 82:8002 {image_name}:{tag}

    docker cp ./entrypoint.sh 3e5a76ecfe48:/opt/workflow/ferry/

    打出来的包缺一个入口文件需要手动塞进去
