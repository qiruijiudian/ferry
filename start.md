<h1>前端启动</h1>

    yarn run dev

<h1>后端启动</h1>

    go run main.go server -c=config/settings.dev.yml

<h1>docker 启动<h1>
    
    docker run -itd --name ferry -v /home/workorder/ferry/config:/opt/workflow/ferry/config -p 82:8002 {image_name}:{tag}

    docker cp ./entrypoint.sh {container_id}:/opt/workflow/ferry/
    docker cp ./email.html {container_id}:/opt/workflow/ferry/static/template/

    打出来的包缺一个入口文件和邮件模板需要手动塞进去

.env.development 填本地 ip 地址
