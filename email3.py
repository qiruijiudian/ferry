

#!/usr/bin/python3
import smtplib
from email.mime.text import MIMEText
from email.header import Header

# 第三方 SMTP 服务
mail_host="smtp.qq.com"  #设置服务器
mail_user="2733396900@qq.com"    #用户名
mail_pass="ojqaiqzqoxasdceg"   #口令
sender = '2733396900@qq.com'
receivers = ['884220564@qq.com']  # 接收邮件，可设置为你的QQ邮箱或者其他邮箱

message = MIMEText('Python 邮件发送测试...', 'plain', 'utf-8')
message['From'] = Header("2733396900@qq.com")
message['To'] =  Header("测试", 'utf-8')

subject = 'Python SMTP 邮件测试'
message['Subject'] = Header(subject, 'utf-8')


try:
    server = smtplib.SMTP_SSL('smtp.qq.com:465')  # smtp.qq.com 的端口是465或587
    server.set_debuglevel(1)
    server.login(mail_user, mail_pass)   # 登录
    server.sendmail(sender,receivers,message.as_string())    # 发送邮件
    server.quit()

    # smtpObj = smtplib.SMTP()
    # smtpObj.connect(mail_host, 465)    # 25 为 SMTP 端口号
    # smtpObj.login(mail_user,mail_pass)
    # smtpObj.sendmail(sender, receivers, message.as_string())
    print ("邮件发送成功")
except smtplib.SMTPException as e:
    print ("Error: 无法发送邮件", e)