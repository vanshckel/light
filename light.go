package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/aws/aws-sdk-go/service/sts"
)

var ERRORFLAG byte = 0
var ERRORNUM int = 0
var ERRORINFO4 string = ""
var ERRORINFO6 string = ""

func main() {
	// AWS 访问密钥 ID 和密钥
	awsAccessKeyID := readInput("密钥: ")
	awsSecretAccessKey := readInput("秘密密钥: ")
	regionName := readInput("区域代码: ")
	number, err := readInt("输入开机数量: ")
	if err != nil {
		// 如果有错误，输出错误提示并退出
		ERRORFLAG = 1
		fmt.Println("输入无效，开机数量请输入一个整数。")
		fmt.Println("程序执行完毕，按任意键退出...")
		fmt.Scanln()
		return
	}

	// 创建AWS会话
	// 创建AWS会话
	sess, accountId, err := createSessionAndCheckCredentials(awsAccessKeyID, awsSecretAccessKey, regionName)
	if err != nil {
		ERRORFLAG = 2
		fmt.Println("创建EC2客户端失败，请检查您的AWS凭证和区域代码是否正确。")
		fmt.Println(err)
		fmt.Println("按任意键退出...")
		fmt.Scanln()
		return
	}

	fmt.Printf("=========账号ID：%d=========\n", accountId)
	ec2Client := ec2.New(sess)

	resp, err := ec2Client.DescribeAvailabilityZones(nil)
	if err != nil {
		ERRORFLAG = 3
		fmt.Println("获取可用区失败:", err)
		fmt.Println("按任意键退出...")
		fmt.Scanln()
		return
	}

	availabilityZones := make([]string, 0)
	for _, zone := range resp.AvailabilityZones {
		availabilityZones = append(availabilityZones, aws.StringValue(zone.ZoneName))
	}

	fmt.Println(availabilityZones)

	availabilityZone := readInput("可用区域（输入空默认随机可用区域）：")
	var randomConf bool
	if availabilityZone == "" {
		fmt.Println("使用默认配置，随机可用区")
		randomConf = true
	}

	lightsailClient := lightsail.New(sess)

	// 创建实例，随机分配可用区
	blueprintID := "ubuntu_22_04" // 使用 Ubuntu 22.04 镜像
	bundleID := "nano_3_0"        // 2h0.5g 实例类型

	for i := 0; i < number; i++ {
		instanceName := fmt.Sprintf("lightsail-instance-%d", i+1)
		var az string
		if randomConf {
			az = availabilityZones[i%len(availabilityZones)]
		} else {
			az = availabilityZone
		}

		_, err := lightsailClient.CreateInstances(&lightsail.CreateInstancesInput{
			InstanceNames:    aws.StringSlice([]string{instanceName}),
			AvailabilityZone: aws.String(az),
			BlueprintId:      aws.String(blueprintID),
			BundleId:         aws.String(bundleID),
			Tags: []*lightsail.Tag{
				{
					Key:   aws.String("Name"),
					Value: aws.String(instanceName),
				},
			},
		})
		if err != nil {
			ERRORFLAG = 4
			ERRORNUM = i
			ERRORINFO4 = err.Error()
			fmt.Printf("创建实例 %s 失败: %v\n", instanceName, err)
			fmt.Printf("已经创建%d台机器\n", i)
			break
		}
		fmt.Printf("创建实例 %s 在 %s 成功\n", instanceName, az)
	}

	fmt.Println("等待实例启动中，20s后检查实例是否启动成功")
	time.Sleep(20 * time.Second)

	instances, err := getAllInstances(lightsailClient)
	if err != nil {
		ERRORFLAG = 5
		fmt.Println("获取实例列表失败:", err)
		fmt.Println("若有已启动的机器，将不会删除")
		fmt.Println("按任意键退出...")
		fmt.Scanln()
		return
	}

	printInstances(instances)

	instancesA := len(instances)

	fmt.Printf("一共成功启动 %d 台机器\n", len(instances))

	fmt.Println("开始删除实例")

	for _, instance := range instances {
		_, err := lightsailClient.DeleteInstance(&lightsail.DeleteInstanceInput{
			InstanceName: instance.Name,
		})
		if err != nil {
			ERRORFLAG = 6
			ERRORINFO6 = err.Error()
			fmt.Printf("删除实例 %s 时出错: %v\n", aws.StringValue(instance.Name), err)
		} else {
			fmt.Printf("已成功删除实例 %s\n", aws.StringValue(instance.Name))
		}
	}

	fmt.Println("等待实例删除完成，20s后检查实例是否删除完成")
	time.Sleep(20 * time.Second)

	instances, err = getAllInstances(lightsailClient)
	if err != nil {
		ERRORFLAG = 7
		fmt.Println("获取实例列表失败:", err)
		fmt.Printf("可能存在未删除实例")
		fmt.Println("程序执行完毕，按任意键退出...")
		fmt.Scanln()
		return
	}

	printInstances(instances)
	instancesB := len(instances)
	fmt.Printf("%s现有 %d 台机器\n", regionName, len(instances))

	fmt.Println("######运行报告总结######")
	switch ERRORFLAG {
	case 4:
		fmt.Println("开机时出现错误")
		fmt.Printf("一共成功启动%d台机器\n", ERRORNUM)
		fmt.Printf("错误原因：%v\n", ERRORINFO4)
	case 6:
		fmt.Println("删除机器时出现错误")
		fmt.Printf("可能存在未删除实例\n错误信息：%v", ERRORINFO6)
	case 0:
		fmt.Println("运行时未发生任何错误")
		fmt.Printf("一共成功启动%d台机器\n删除后检查剩余%d台机器", instancesA, instancesB)
	}

	fmt.Println("程序执行完毕，按任意键退出...")
	fmt.Scanln()
}

func getAllInstances(client *lightsail.Lightsail) ([]*lightsail.Instance, error) {
	instances := make([]*lightsail.Instance, 0)
	params := &lightsail.GetInstancesInput{}

	for {
		output, err := client.GetInstances(params)
		if err != nil {
			return nil, err
		}
		for _, instance := range output.Instances {
			instances = append(instances, instance)
		}

		if output.NextPageToken == nil {
			break
		}
		params.PageToken = output.NextPageToken
	}
	return instances, nil
}

func printInstances(instances []*lightsail.Instance) {
	for _, instance := range instances {
		publicIP := "N/A"
		if instance.PublicIpAddress != nil {
			publicIP = aws.StringValue(instance.PublicIpAddress)
		}
		fmt.Println(aws.StringValue(instance.Name), publicIP, aws.StringValue(instance.State.Name))
	}
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Scanln(&input)
	return input
}

func readInt(prompt string) (int, error) {
	// 打印提示信息
	fmt.Print(prompt)
	var input string
	// 读取用户输入
	fmt.Scanln(&input)

	// 尝试将输入转换为整数
	number, err := strconv.Atoi(input)
	if err != nil {
		// 如果转换失败，返回错误
		return 0, err
	}

	// 成功返回整数和nil错误
	return number, nil
}

func createSession(awsAccessKeyID, awsSecretAccessKey, regionName string) (*session.Session, error) {
	config := aws.Config{
		Region:      aws.String(regionName),
		Credentials: credentials.NewStaticCredentials(awsAccessKeyID, awsSecretAccessKey, ""),
	}

	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, err
	}

	// 验证凭证是否有效
	_, err = sess.Config.Credentials.Get()
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NoCredentialProviders" {
			return nil, fmt.Errorf("未找到凭证")
		}
		return nil, fmt.Errorf("获取凭证失败")
	}

	return sess, nil
}

func createSessionAndCheckCredentials(awsAccessKeyID, awsSecretAccessKey, regionName string) (*session.Session, *string, error) {
	config := aws.Config{
		Region:      aws.String(regionName),
		Credentials: credentials.NewStaticCredentials(awsAccessKeyID, awsSecretAccessKey, ""),
	}

	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, nil, err
	}

	// 验证凭证是否有效，并获取账号ID
	stsSvc := sts.New(sess)
	getCallerIdentityInput := &sts.GetCallerIdentityInput{}
	getCallerIdentityOutput, err := stsSvc.GetCallerIdentity(getCallerIdentityInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NoCredentialProviders" {
			return nil, nil, fmt.Errorf("未找到凭证")
		}
		return nil, nil, fmt.Errorf("获取凭证失败")
	}

	return sess, getCallerIdentityOutput.Account, nil
}
